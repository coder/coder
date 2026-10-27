import { ArrowUpRightIcon } from "lucide-react";
import type * as TypesGen from "#/api/typesGenerated";
import { ErrorAlert } from "#/components/Alert/ErrorAlert";
import { Button } from "#/components/Button/Button";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { OrganizationField } from "#/components/OrganizationAutocomplete/OrganizationAutocomplete";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { Spinner } from "#/components/Spinner/Spinner";
import {
	StatusIndicator,
	StatusIndicatorDot,
} from "#/components/StatusIndicator/StatusIndicator";
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
import { RetryButton } from "../../components/RetryButton";
import { SectionHeader } from "../../components/SectionHeader";

type AgentSettingsMCPConnectionsPageViewProps = {
	organizations: readonly TypesGen.Organization[];
	organization: TypesGen.Organization | undefined;
	onOrganizationChange: (organization: TypesGen.Organization) => void;
	/** Enabled servers of the selected organization, in API order. */
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

const listFormat = new Intl.ListFormat("en-US", {
	style: "long",
	type: "conjunction",
});

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
	const organizationName = organization
		? organization.display_name || organization.name
		: "";
	const enabledServers = servers?.filter((server) => server.enabled) ?? [];
	const oauthServers = enabledServers.filter(
		(server) => server.auth_type === "oauth2",
	);
	const noSignInNames = enabledServers
		.filter((server) => server.auth_type !== "oauth2")
		.map((server) => server.display_name);

	return (
		<div className="flex flex-col gap-8">
			<SectionHeader
				label="MCP connections"
				description="Sign in to the MCP servers your organization offers. Connected servers appear in the chat's + menu under MCP servers, where you turn them on for each chat."
			/>
			{organization && (
				<OrganizationField
					id="mcp-connections-organization"
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
			<div className="flex flex-col gap-4">
				<Table aria-label={`MCP connections in ${organizationName}`}>
					<TableHeader>
						<TableRow>
							<TableHead className="whitespace-nowrap">Name</TableHead>
							<TableHead className="w-full">Description</TableHead>
							<TableHead className="whitespace-nowrap">Status</TableHead>
							<TableHead>
								<span className="sr-only">Actions</span>
							</TableHead>
						</TableRow>
					</TableHeader>
					<TableBody size="lg">
						{isLoading ? (
							<TableLoaderSkeleton>
								<TableRowSkeleton aria-label="Loading MCP servers">
									<TableCell>
										<Skeleton variant="text" className="w-32" />
									</TableCell>
									<TableCell className="w-full max-w-0">
										<Skeleton variant="text" />
									</TableCell>
									<TableCell>
										<Skeleton variant="text" className="w-24" />
									</TableCell>
									<TableCell>
										<Skeleton className="h-8 w-24" />
									</TableCell>
								</TableRowSkeleton>
							</TableLoaderSkeleton>
						) : oauthServers.length === 0 ? (
							<TableEmpty
								message={
									error
										? "Failed to load MCP servers"
										: `No MCP servers need sign-in in ${organizationName}`
								}
							/>
						) : (
							oauthServers.map((server) => {
								const isConnecting = connectingServerId === server.id;
								return (
									<TableRow key={server.id}>
										<TableCell className="max-w-48">
											<div className="flex min-w-0 items-center gap-2">
												<MCPServerIcon
													iconUrl={server.icon_url}
													className="size-6"
												/>
												<span className="truncate" title={server.display_name}>
													{server.display_name}
												</span>
											</div>
										</TableCell>
										<TableCell
											className="w-full max-w-0 truncate"
											title={server.description || undefined}
										>
											{server.description || (
												<span className="text-content-disabled">
													No description
												</span>
											)}
										</TableCell>
										<TableCell className="whitespace-nowrap">
											{isConnecting ? (
												<StatusIndicator variant="pending" size="sm">
													<StatusIndicatorDot />
													Waiting for sign-in
												</StatusIndicator>
											) : server.auth_connected ? (
												<StatusIndicator variant="success" size="sm">
													<StatusIndicatorDot />
													Connected
												</StatusIndicator>
											) : (
												<StatusIndicator variant="inactive" size="sm">
													<StatusIndicatorDot />
													Not connected
												</StatusIndicator>
											)}
										</TableCell>
										<TableCell className="text-right">
											{server.auth_connected ? (
												<Button
													variant="outline"
													size="sm"
													aria-label={`Disconnect ${server.display_name}`}
													onClick={() => onDisconnect(server)}
												>
													Disconnect
												</Button>
											) : (
												<Button
													variant="outline"
													size="sm"
													aria-label={`Connect ${server.display_name}`}
													disabled={connectingServerId !== null}
													onClick={() => onConnect(server)}
												>
													{isConnecting ? (
														<>
															<Spinner loading className="size-4" />
															Connecting
														</>
													) : (
														<>
															Connect
															<ArrowUpRightIcon />
														</>
													)}
												</Button>
											)}
										</TableCell>
									</TableRow>
								);
							})
						)}
					</TableBody>
				</Table>
				{noSignInNames.length > 0 && (
					<p className="m-0 text-sm text-content-secondary">
						{noSignInNames.length === 1
							? `${noSignInNames[0]} needs no sign-in, so it is always listed in the + menu.`
							: `${listFormat.format(noSignInNames)} need no sign-in, so they are always listed in the + menu.`}
					</p>
				)}
			</div>
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
