import { LockIcon, ServerIcon, UnlinkIcon } from "lucide-react";
import type { FC } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";

interface MCPServerToggleListProps {
	servers: readonly TypesGen.MCPServerConfig[];
	selectedServerIds: readonly string[] | undefined;
	onToggle: (serverId: string, checked: boolean) => void;
	onConnect: (serverId: string) => void;
	connectingServerId: string | null;
	onDisconnect: (server: TypesGen.MCPServerConfig) => void;
	isDisabled?: boolean;
}

export const MCPServerToggleList: FC<MCPServerToggleListProps> = ({
	servers,
	selectedServerIds,
	onToggle,
	onConnect,
	connectingServerId,
	onDisconnect,
	isDisabled,
}) => (
	<>
		{servers.map((server) => {
			const isForceOn = server.availability === "force_on";
			const isSelected =
				isForceOn || (selectedServerIds?.includes(server.id) ?? false);
			const needsAuth = server.auth_type === "oauth2" && !server.auth_connected;
			const isConnecting = connectingServerId === server.id;
			return (
				<div key={server.id} className="flex items-center gap-1.5 px-1 py-1.5">
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
					{needsAuth ? (
						<Button
							variant="outline"
							size="sm"
							className="h-6 shrink-0 px-2 text-[10px] leading-none"
							onClick={() => onConnect(server.id)}
							disabled={isDisabled || connectingServerId !== null}
						>
							{isConnecting ? (
								<Spinner loading className="h-2.5 w-2.5" />
							) : null}
							Auth
						</Button>
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
							{isForceOn && (
								<LockIcon className="size-3 shrink-0 text-content-secondary" />
							)}
							<Switch
								size="sm"
								checked={isSelected}
								onCheckedChange={(checked) => onToggle(server.id, checked)}
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
		})}
	</>
);
