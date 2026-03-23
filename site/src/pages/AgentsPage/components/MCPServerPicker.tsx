import type * as TypesGen from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { Button } from "components/Button/Button";
import { ExternalImage } from "components/ExternalImage/ExternalImage";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "components/Popover/Popover";
import { Spinner } from "components/Spinner/Spinner";
import { Switch } from "components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import {
	CheckCircleIcon,
	ExternalLinkIcon,
	LockIcon,
	PlugIcon,
	ServerIcon,
} from "lucide-react";
import { type FC, useCallback, useEffect, useRef, useState } from "react";
import { cn } from "utils/cn";

// ── Types ──────────────────────────────────────────────────────

export interface MCPServerPickerProps {
	/** All MCP server configs from the API. Will be filtered to enabled only. */
	servers: readonly TypesGen.MCPServerConfig[];
	/** Currently selected server IDs. */
	selectedServerIds: readonly string[];
	/** Called when the user toggles a server. */
	onSelectionChange: (ids: string[]) => void;
	/** Called when an OAuth2 auth flow completes (server should be refetched). */
	onAuthComplete: (serverId: string) => void;
	/** Whether the picker is disabled (e.g. during submission). */
	disabled?: boolean;
}

// ── Helpers ────────────────────────────────────────────────────

const MCPIcon: FC<{ iconUrl: string; name: string; className?: string }> = ({
	iconUrl,
	name,
	className,
}) => {
	if (iconUrl) {
		return (
			<div
				className={cn(
					"flex shrink-0 items-center justify-center rounded-full bg-surface-secondary",
					className,
				)}
			>
				<ExternalImage
					src={iconUrl}
					alt={`${name} icon`}
					className="h-3/5 w-3/5"
				/>
			</div>
		);
	}
	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-center rounded-full bg-surface-secondary",
				className,
			)}
		>
			<ServerIcon className="h-3/5 w-3/5 text-content-secondary" />
		</div>
	);
};

/**
 * Compute the default selection based on server availability policies.
 * force_on and default_on servers are selected by default.
 */
export const getDefaultMCPSelection = (
	servers: readonly TypesGen.MCPServerConfig[],
): string[] => {
	return servers
		.filter(
			(s) =>
				s.enabled &&
				(s.availability === "force_on" || s.availability === "default_on"),
		)
		.map((s) => s.id);
};

// ── Component ──────────────────────────────────────────────────

export const MCPServerPicker: FC<MCPServerPickerProps> = ({
	servers,
	selectedServerIds,
	onSelectionChange,
	onAuthComplete,
	disabled = false,
}) => {
	const [open, setOpen] = useState(false);
	const [connectingServerId, setConnectingServerId] = useState<string | null>(
		null,
	);
	const popupRef = useRef<Window | null>(null);

	// Filter to enabled servers only.
	const enabledServers = servers.filter((s) => s.enabled);

	// Listen for OAuth2 completion postMessage from popup.
	useEffect(() => {
		const handler = (event: MessageEvent) => {
			if (
				event.data?.type === "mcp-oauth2-complete" &&
				typeof event.data.serverID === "string"
			) {
				setConnectingServerId(null);
				onAuthComplete(event.data.serverID);
				popupRef.current = null;
			}
		};
		window.addEventListener("message", handler);
		return () => window.removeEventListener("message", handler);
	}, [onAuthComplete]);

	// Clean up popup ref when it closes.
	useEffect(() => {
		if (!connectingServerId || !popupRef.current) return;
		const interval = setInterval(() => {
			if (popupRef.current?.closed) {
				setConnectingServerId(null);
				popupRef.current = null;
			}
		}, 500);
		return () => clearInterval(interval);
	}, [connectingServerId]);

	const handleToggle = useCallback(
		(serverId: string, checked: boolean) => {
			if (checked) {
				onSelectionChange([...selectedServerIds, serverId]);
			} else {
				onSelectionChange(selectedServerIds.filter((id) => id !== serverId));
			}
		},
		[selectedServerIds, onSelectionChange],
	);

	const handleConnect = useCallback((server: TypesGen.MCPServerConfig) => {
		setConnectingServerId(server.id);
		const connectUrl = `/api/experimental/mcp/servers/${encodeURIComponent(server.id)}/oauth2/connect`;
		popupRef.current = window.open(
			connectUrl,
			"_blank",
			"width=900,height=600",
		);
	}, []);

	if (enabledServers.length === 0) {
		return null;
	}

	const activeCount = enabledServers.filter(
		(s) => s.availability === "force_on" || selectedServerIds.includes(s.id),
	).length;

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<Tooltip>
				<TooltipTrigger asChild>
					<PopoverTrigger asChild>
						<Button
							variant="outline"
							size="sm"
							className="h-7 gap-1.5 text-xs"
							disabled={disabled}
							aria-label="MCP Servers"
						>
							<PlugIcon className="h-3.5 w-3.5" />
							<span className="hidden sm:inline">MCP</span>
							{activeCount > 0 && (
								<Badge
									size="sm"
									border="none"
									className="ml-0 min-w-[18px] justify-center px-1 py-0 text-[10px]"
								>
									{activeCount}
								</Badge>
							)}
						</Button>
					</PopoverTrigger>
				</TooltipTrigger>
				<TooltipContent>Configure MCP Servers</TooltipContent>
			</Tooltip>
			<PopoverContent align="start" className="w-80 p-0">
				<div className="border-0 border-b border-solid border-border px-3 py-2">
					<h4 className="m-0 text-sm font-medium text-content-primary">
						MCP Servers
					</h4>
					<p className="m-0 text-xs text-content-secondary">
						Select which servers provide tools for this chat.
					</p>
				</div>
				<div className="max-h-64 overflow-y-auto py-1 [scrollbar-width:thin]">
					{enabledServers.map((server) => {
						const isForceOn = server.availability === "force_on";
						const isSelected =
							isForceOn || selectedServerIds.includes(server.id);
						const needsAuth =
							server.auth_type === "oauth2" && !server.auth_connected;
						const isConnecting = connectingServerId === server.id;

						return (
							<div
								key={server.id}
								className="flex items-center gap-3 px-3 py-2"
							>
								<MCPIcon
									iconUrl={server.icon_url}
									name={server.display_name}
									className="h-7 w-7"
								/>
								<div className="min-w-0 flex-1">
									<div className="flex items-center gap-1.5">
										<span className="truncate text-sm font-medium text-content-primary">
											{server.display_name}
										</span>
										{isForceOn && (
											<Tooltip>
												<TooltipTrigger asChild>
													<LockIcon className="h-3 w-3 shrink-0 text-content-secondary" />
												</TooltipTrigger>
												<TooltipContent>
													This server is always enabled by your administrator.
												</TooltipContent>
											</Tooltip>
										)}
									</div>
									{server.description && (
										<p className="m-0 truncate text-xs text-content-secondary">
											{server.description}
										</p>
									)}
									{needsAuth && isSelected && (
										<button
											type="button"
											className={cn(
												"mt-1 inline-flex cursor-pointer items-center gap-1 rounded border-0 bg-transparent p-0 text-xs font-medium transition-colors",
												isConnecting
													? "text-content-secondary"
													: "text-content-link hover:text-content-link/80",
											)}
											onClick={() => handleConnect(server)}
											disabled={isConnecting}
										>
											{isConnecting ? (
												<>
													<Spinner loading className="h-3 w-3" />
													Connecting...
												</>
											) : (
												<>
													<ExternalLinkIcon className="h-3 w-3" />
													Connect to authenticate
												</>
											)}
										</button>
									)}
									{server.auth_type === "oauth2" &&
										server.auth_connected &&
										isSelected && (
											<span className="mt-0.5 inline-flex items-center gap-1 text-xs text-content-success">
												<CheckCircleIcon className="h-3 w-3" />
												Connected
											</span>
										)}
								</div>
								<Switch
									checked={isSelected}
									onCheckedChange={(checked) =>
										handleToggle(server.id, checked)
									}
									disabled={disabled || isForceOn}
									aria-label={`${isSelected ? "Disable" : "Enable"} ${server.display_name}`}
								/>
							</div>
						);
					})}
				</div>
			</PopoverContent>
		</Popover>
	);
};
