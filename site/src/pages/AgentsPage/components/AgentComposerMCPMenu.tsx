import { LockIcon, ServerIcon, UnlinkIcon } from "lucide-react";
import type { MCPServerConfig } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import type { AgentComposerOptionsData } from "./AgentComposerOptionsContext";

/** Servers the deployment has enabled; disabled servers are never offered. */
export const enabledMcpServers = (
	mcp: AgentComposerOptionsData["mcp"],
): MCPServerConfig[] => mcp?.servers.filter((server) => server.enabled) ?? [];

/** Updates controlled MCP selection without changing unrelated server IDs. */
export function setMCPServerSelected(
	mcp: Pick<
		NonNullable<AgentComposerOptionsData["mcp"]>,
		"selectedServerIds" | "onSelectionChange"
	>,
	serverId: string,
	checked: boolean,
) {
	mcp.onSelectionChange(
		checked
			? [...mcp.selectedServerIds, serverId]
			: mcp.selectedServerIds.filter((id) => id !== serverId),
	);
}

type MCPServerMenuItemProps = {
	server: MCPServerConfig;
	mcp: NonNullable<AgentComposerOptionsData["mcp"]>;
	connectingServerId: string | null;
	isDisabled: boolean;
	onConnect: (id: string) => void;
	onDisconnect: (server: MCPServerConfig) => void;
};

/** MCP authentication actions and controlled selection for one server. */
export const MCPServerMenuItem = ({
	server,
	mcp,
	connectingServerId,
	isDisabled,
	onConnect,
	onDisconnect,
}: MCPServerMenuItemProps) => {
	const isForceOn = server.availability === "force_on";
	const isSelected = isForceOn || mcp.selectedServerIds.includes(server.id);

	const needsAuth = server.auth_type === "oauth2" && !server.auth_connected;
	const isConnecting = connectingServerId === server.id;

	return (
		<div className="flex items-center gap-1.5 px-1 py-1.5">
			{server.icon_url ? (
				<ExternalImage
					src={server.icon_url}
					alt=""
					className="size-3.5 shrink-0 rounded-sm"
				/>
			) : (
				<ServerIcon className="size-3.5 shrink-0 text-content-secondary" />
			)}
			<span className="min-w-0 flex-1 truncate text-xs text-content-secondary">
				{server.display_name}
			</span>
			{isForceOn && (
				<LockIcon className="size-3 shrink-0 text-content-secondary" />
			)}
			{needsAuth ? (
				<>
					{isForceOn && <span className="sr-only">Always on</span>}
					<Button
						variant="outline"
						size="sm"
						className="h-6 shrink-0 px-2 text-[10px] leading-none"
						onClick={() => onConnect(server.id)}
						disabled={isDisabled || connectingServerId !== null}
					>
						{isConnecting ? <Spinner loading className="h-2.5 w-2.5" /> : null}
						Auth
					</Button>
				</>
			) : (
				<>
					{server.auth_type === "oauth2" && (
						<Button
							variant="subtle"
							size="icon"
							className="size-6 shrink-0 text-content-secondary [&>svg]:size-3"
							onClick={() => onDisconnect(server)}
							disabled={isDisabled}
							aria-label={`Disconnect ${server.display_name}`}
						>
							<UnlinkIcon />
						</Button>
					)}
					<Switch
						size="sm"
						checked={isSelected}
						onCheckedChange={(checked) =>
							setMCPServerSelected(mcp, server.id, checked)
						}
						disabled={isDisabled || isForceOn}
						aria-label={
							isForceOn
								? `${server.display_name} always on`
								: `${isSelected ? "Disable" : "Enable"} ${server.display_name}`
						}
					/>
				</>
			)}
		</div>
	);
};
