import { useId } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { OrganizationField } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import { StatusIndicatorDot } from "#/components/StatusIndicator/StatusIndicator";
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from "#/components/Table/Table";
import { TableEmpty } from "#/components/TableEmpty/TableEmpty";
import {
	TableLoaderSkeleton,
	TableRowSkeleton,
} from "#/components/TableLoader/TableLoader";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { MCPServerIcon } from "#/modules/mcpServers/MCPServerIcon";
import { RetryButton } from "../../components/RetryButton";
import { SectionHeader } from "../../components/SectionHeader";

type AgentSettingsMCPConnectionsPageViewProps = {
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization | undefined;
	onOrganizationChange: (organization: TypesGen.Organization) => void;
	servers: readonly TypesGen.MCPServerConfig[] | undefined;
	error: unknown;
	isLoading: boolean;
	isRetrying: boolean;
	onRetry: () => void;
	connectingServerId: string | null;
	onConnect: (server: TypesGen.MCPServerConfig) => void;
	onDisconnect: (server: TypesGen.MCPServerConfig) => void;
	disconnectTarget: TypesGen.MCPServerConfig | null;
	isDisconnecting: boolean;
	onConfirmDisconnect: () => void;
	onCancelDisconnect: () => void;
};

type ConnectionStatusProps = {
	isConnecting: boolean;
	isConnected: boolean;
};

const ConnectionStatus: React.FC<ConnectionStatusProps> = ({
	isConnecting,
	isConnected,
}) => {
	let variant: "pending" | "success" | "inactive" = "inactive";
	let label = "Not connected";
	if (isConnecting) {
		variant = "pending";
		label = "Waiting for sign-in";
	} else if (isConnected) {
		variant = "success";
		label = "Connected";
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<StatusIndicatorDot
					role="img"
					aria-label={label}
					variant={variant}
					size="sm"
				/>
			</TooltipTrigger>
			<TooltipContent>{label}</TooltipContent>
		</Tooltip>
	);
};

type ConnectionActionProps = {
	server: TypesGen.MCPServerConfig;
	isConnecting: boolean;
	isConnectDisabled: boolean;
	onConnect: (server: TypesGen.MCPServerConfig) => void;
	onDisconnect: (server: TypesGen.MCPServerConfig) => void;
};

const ConnectionAction: React.FC<ConnectionActionProps> = ({
	server,
	isConnecting,
	isConnectDisabled,
	onConnect,
	onDisconnect,
}) => {
	if (server.auth_type !== "oauth2") {
		return (
			<span className="whitespace-nowrap text-content-secondary">
				No auth required
			</span>
		);
	}

	if (server.auth_connected) {
		return (
			<Button
				variant="outline"
				size="sm"
				aria-label={`Disconnect ${server.display_name}`}
				onClick={() => onDisconnect(server)}
			>
				Disconnect
			</Button>
		);
	}

	const connectLabel = isConnecting ? "Connecting" : "Connect";
	return (
		<Button
			variant="outline"
			size="sm"
			aria-label={`${connectLabel} ${server.display_name}`}
			disabled={isConnectDisabled}
			onClick={() => onConnect(server)}
		>
			<Spinner loading={isConnecting} className="size-4" />
			{connectLabel}
		</Button>
	);
};

type MCPConnectionRowProps = {
	server: TypesGen.MCPServerConfig;
	connectingServerId: string | null;
	onConnect: (server: TypesGen.MCPServerConfig) => void;
	onDisconnect: (server: TypesGen.MCPServerConfig) => void;
};

const MCPConnectionRow: React.FC<MCPConnectionRowProps> = ({
	server,
	connectingServerId,
	onConnect,
	onDisconnect,
}) => {
	const isConnecting = connectingServerId === server.id;

	return (
		<TableRow>
			<TableCell className="w-full max-w-0 @lg:w-auto @lg:max-w-48">
				<div className="flex items-center gap-2">
					<MCPServerIcon iconUrl={server.icon_url} className="size-6" />
					<span className="truncate" title={server.display_name}>
						{server.display_name}
					</span>
				</div>
			</TableCell>
			<TableCell
				className="hidden w-full max-w-0 truncate @lg:table-cell"
				title={server.description || undefined}
			>
				{server.description || (
					<span className="text-content-disabled">No description</span>
				)}
			</TableCell>
			<TableCell className="hidden whitespace-nowrap @2xl:table-cell">
				{server.auth_type === "oauth2" && (
					<ConnectionStatus
						isConnecting={isConnecting}
						isConnected={server.auth_connected}
					/>
				)}
			</TableCell>
			<TableCell className="text-right">
				<ConnectionAction
					server={server}
					isConnecting={isConnecting}
					isConnectDisabled={connectingServerId !== null}
					onConnect={onConnect}
					onDisconnect={onDisconnect}
				/>
			</TableCell>
		</TableRow>
	);
};

/**
 * Rows list the enabled servers, OAuth2 ones first; only those offer Connect
 * or Disconnect.
 * `onDisconnect` should set `disconnectTarget`, which opens the confirmation,
 * and while `connectingServerId` is set, every Connect button is disabled.
 */
export const AgentSettingsMCPConnectionsPageView: React.FC<
	AgentSettingsMCPConnectionsPageViewProps
> = ({
	organizations,
	organization,
	onOrganizationChange,
	servers,
	error,
	isLoading,
	isRetrying,
	onRetry,
	connectingServerId,
	onConnect,
	onDisconnect,
	disconnectTarget,
	isDisconnecting,
	onConfirmDisconnect,
	onCancelDisconnect,
}) => {
	const organizationFieldId = useId();
	const organizationName = organization
		? organization.display_name || organization.name
		: "your organization";
	const emptyMessage = organization
		? `No MCP servers in ${organizationName}`
		: "You are not a member of any organization";

	const enabledServers = servers?.filter((server) => server.enabled) ?? [];
	const rows = [
		...enabledServers.filter((server) => server.auth_type === "oauth2"),
		...enabledServers.filter((server) => server.auth_type !== "oauth2"),
	];
	const failedFirstLoad = Boolean(error) && servers === undefined;

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="MCP connections"
				description="Sign in to the MCP servers your organization offers. Connected servers appear in the chat's + menu under MCP servers, where you turn them on for each chat."
			/>
			{organization && (
				<OrganizationField
					id={organizationFieldId}
					className="w-full sm:w-60"
					organizations={organizations}
					organization={organization}
					onChange={onOrganizationChange}
					showLabel={false}
				/>
			)}
			{Boolean(error) && (
				<ErrorAlert
					error={error}
					actions={
						<RetryButton
							size="sm"
							variant="outline"
							aria-label={`Retry loading ${organizationName} MCP servers`}
							isRetrying={isRetrying}
							onRetry={onRetry}
						/>
					}
				/>
			)}
			{!failedFirstLoad && (
				<Table
					aria-label={`MCP connections in ${organizationName}`}
					// Beside the settings sidebar the table is far narrower than the
					// viewport, so the columns follow the wrapper, not the screen.
					wrapperClassName="@container"
				>
					<TableHeader>
						<TableRow>
							<TableHead className="whitespace-nowrap">Name</TableHead>
							<TableHead className="hidden w-full @lg:table-cell">
								Description
							</TableHead>
							<TableHead className="hidden whitespace-nowrap @2xl:table-cell">
								Status
							</TableHead>
							<TableHead>
								<span className="sr-only">Actions</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody size="lg">
						{isLoading ? (
							<TableLoaderSkeleton>
								<TableRowSkeleton aria-label="Loading MCP servers">
									<TableCell className="w-full max-w-0 @lg:w-auto @lg:max-w-48">
										<Skeleton variant="text" className="w-32" />
									</TableCell>
									<TableCell className="hidden w-full max-w-0 @lg:table-cell">
										<Skeleton variant="text" />
									</TableCell>
									<TableCell className="hidden @2xl:table-cell">
										<Skeleton variant="text" className="w-24" />
									</TableCell>
									<TableCell>
										<Skeleton className="ml-auto h-8 w-24" />
									</TableCell>
								</TableRowSkeleton>
							</TableLoaderSkeleton>
						) : rows.length === 0 ? (
							<TableEmpty message={emptyMessage} />
						) : (
							rows.map((server) => (
								<MCPConnectionRow
									key={server.id}
									server={server}
									connectingServerId={connectingServerId}
									onConnect={onConnect}
									onDisconnect={onDisconnect}
								/>
							))
						)}
					</TableBody>
				</Table>
			)}
			<ConfirmDialog
				open={disconnectTarget !== null}
				title={`Disconnect ${disconnectTarget?.display_name ?? "MCP server"}?`}
				description="This removes your credentials for this MCP server from Coder. You can authenticate again later."
				type="delete"
				confirmText="Disconnect"
				confirmLoading={isDisconnecting}
				onConfirm={onConfirmDisconnect}
				onClose={onCancelDisconnect}
			/>
		</div>
	);
};
