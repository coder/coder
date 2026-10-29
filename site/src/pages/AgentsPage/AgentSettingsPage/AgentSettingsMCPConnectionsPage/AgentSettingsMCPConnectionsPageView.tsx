import { ArrowDownIcon } from "lucide-react";
import { useId, useRef } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { OrganizationField } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
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
import { MCPServerIcon } from "#/modules/mcpServers/MCPServerIcon";
import { needsSignIn } from "../../components/mcpMenuServers";
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

type AddedConnectorActionProps = {
	server: TypesGen.MCPServerConfig;
	onDisconnect: (server: TypesGen.MCPServerConfig) => void;
};

const AddedConnectorAction: React.FC<AddedConnectorActionProps> = ({
	server,
	onDisconnect,
}) => {
	if (server.auth_type !== "oauth2") {
		return (
			<span className="whitespace-nowrap text-content-secondary">
				No auth required
			</span>
		);
	}

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
};

type AddedConnectorRowProps = {
	server: TypesGen.MCPServerConfig;
	onDisconnect: (server: TypesGen.MCPServerConfig) => void;
};

const AddedConnectorRow: React.FC<AddedConnectorRowProps> = ({
	server,
	onDisconnect,
}) => {
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
			<TableCell className="text-right">
				<AddedConnectorAction server={server} onDisconnect={onDisconnect} />
			</TableCell>
		</TableRow>
	);
};

type AvailableConnectorCardProps = {
	server: TypesGen.MCPServerConfig;
	connectingServerId: string | null;
	onConnect: (server: TypesGen.MCPServerConfig) => void;
};

const AvailableConnectorCard: React.FC<AvailableConnectorCardProps> = ({
	server,
	connectingServerId,
	onConnect,
}) => {
	const isConnecting = connectingServerId === server.id;
	const connectLabel = isConnecting ? "Connecting" : "Connect";

	return (
		<li className="flex flex-col gap-3 rounded-lg border border-solid border-border p-4">
			<div className="flex items-center gap-3">
				<MCPServerIcon iconUrl={server.icon_url} className="size-8" />
				<span
					className="truncate font-medium text-content-primary"
					title={server.display_name}
				>
					{server.display_name}
				</span>
			</div>
			<p
				className="m-0 line-clamp-2 text-sm text-content-secondary break-words"
				title={server.description || undefined}
			>
				{server.description || (
					<span className="text-content-disabled">No description</span>
				)}
			</p>
			<Button
				variant="outline"
				size="sm"
				className="mt-auto self-end"
				aria-label={`${connectLabel} ${server.display_name}`}
				disabled={connectingServerId !== null}
				onClick={() => onConnect(server)}
			>
				<Spinner loading={isConnecting} className="size-4" />
				{connectLabel}
			</Button>
		</li>
	);
};

type AddedConnectorsSectionProps = {
	organizationName: string;
	servers: readonly TypesGen.MCPServerConfig[];
	enabledCount: number;
	availableCount: number;
	isLoading: boolean;
	emptyMessage: string;
	onShowAvailable: () => void;
	onDisconnect: (server: TypesGen.MCPServerConfig) => void;
};

const AddedConnectorsSection: React.FC<AddedConnectorsSectionProps> = ({
	organizationName,
	servers,
	enabledCount,
	availableCount,
	isLoading,
	emptyMessage,
	onShowAvailable,
	onDisconnect,
}) => {
	const showCounts = !isLoading && enabledCount > 0;

	let body: React.ReactNode = <TableEmpty message={emptyMessage} />;
	if (isLoading) {
		body = (
			<TableLoaderSkeleton>
				<TableRowSkeleton aria-label="Loading MCP servers">
					<TableCell className="w-full max-w-0 @lg:w-auto @lg:max-w-48">
						<Skeleton variant="text" className="w-32" />
					</TableCell>
					<TableCell className="hidden w-full max-w-0 @lg:table-cell">
						<Skeleton variant="text" />
					</TableCell>
					<TableCell>
						<Skeleton className="ml-auto h-8 w-24" />
					</TableCell>
				</TableRowSkeleton>
			</TableLoaderSkeleton>
		);
	} else if (servers.length > 0) {
		body = servers.map((server) => (
			<AddedConnectorRow
				key={server.id}
				server={server}
				onDisconnect={onDisconnect}
			/>
		));
	}

	return (
		<section aria-label="Added connectors" className="flex flex-col gap-4">
			<SectionHeader
				level="section"
				label="Added connectors"
				badge={
					showCounts && (
						<Badge size="sm">
							{servers.length} of {enabledCount}
						</Badge>
					)
				}
				description="Servers you've connected, plus ones that need no sign-in. They appear in the chat's + menu."
				action={
					showCounts &&
					availableCount > 0 && (
						<Button variant="outline" size="sm" onClick={onShowAvailable}>
							View {availableCount} available
							<ArrowDownIcon />
						</Button>
					)
				}
			/>
			<Table
				aria-label={`Added connectors in ${organizationName}`}
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
						<TableHead>
							<span className="sr-only">Actions</span>
						</TableHead>
					</TableRow>
				</TableHeader>
				<TableBody size="lg">{body}</TableBody>
			</Table>
		</section>
	);
};

type AvailableConnectorsSectionProps = {
	ref: React.Ref<HTMLElement>;
	servers: readonly TypesGen.MCPServerConfig[];
	connectingServerId: string | null;
	onConnect: (server: TypesGen.MCPServerConfig) => void;
};

const AvailableConnectorsSection: React.FC<AvailableConnectorsSectionProps> = ({
	ref,
	servers,
	connectingServerId,
	onConnect,
}) => {
	return (
		<section
			ref={ref}
			aria-label="Available connectors"
			tabIndex={-1}
			className="flex scroll-mt-8 flex-col gap-4 outline-hidden"
		>
			<SectionHeader
				level="section"
				label="Available connectors"
				badge={<Badge size="sm">{servers.length}</Badge>}
				description="Servers your organization offers. Connect to one to add it to chat."
			/>
			<ul className="m-0 grid list-none grid-cols-1 gap-3 p-0 @lg:grid-cols-2 @3xl:grid-cols-3">
				{servers.map((server) => (
					<AvailableConnectorCard
						key={server.id}
						server={server}
						connectingServerId={connectingServerId}
						onConnect={onConnect}
					/>
				))}
			</ul>
		</section>
	);
};

/**
 * The table lists the enabled servers a chat can use now; the OAuth2 servers
 * the user has not signed in to are cards below it, each with Connect.
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
	const availableSectionRef = useRef<HTMLElement>(null);
	const organizationName = organization
		? organization.display_name || organization.name
		: "your organization";

	const enabledServers = servers?.filter((server) => server.enabled) ?? [];
	const availableServers = enabledServers.filter(needsSignIn);
	const addedServers = [
		...enabledServers.filter(
			(server) => server.auth_type === "oauth2" && server.auth_connected,
		),
		...enabledServers.filter((server) => server.auth_type !== "oauth2"),
	];
	const failedFirstLoad = Boolean(error) && servers === undefined;

	let emptyMessage = "Connect a server below to add it to chat";
	if (!organization) {
		emptyMessage = "You are not a member of any organization";
	} else if (enabledServers.length === 0) {
		emptyMessage = `No MCP servers in ${organizationName}`;
	}

	const showAvailableServers = () => {
		const section = availableSectionRef.current;
		section?.scrollIntoView({ behavior: "smooth", block: "start" });
		section?.focus({ preventScroll: true });
	};

	return (
		<div className="flex flex-col gap-8 @container">
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
				<AddedConnectorsSection
					organizationName={organizationName}
					servers={addedServers}
					enabledCount={enabledServers.length}
					availableCount={availableServers.length}
					isLoading={isLoading}
					emptyMessage={emptyMessage}
					onShowAvailable={showAvailableServers}
					onDisconnect={onDisconnect}
				/>
			)}
			{!isLoading && availableServers.length > 0 && (
				<AvailableConnectorsSection
					ref={availableSectionRef}
					servers={availableServers}
					connectingServerId={connectingServerId}
					onConnect={onConnect}
				/>
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
