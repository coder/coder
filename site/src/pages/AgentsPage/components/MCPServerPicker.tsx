import { ChevronDownIcon, LockIcon, ServerIcon } from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router";
import type * as TypesGen from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { Spinner } from "#/components/Spinner/Spinner";
import { Switch } from "#/components/Switch/Switch";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "#/components/Tooltip/Tooltip";
import { cn } from "#/utils/cn";

// ── Types ──────────────────────────────────────────────────────

interface MCPServerPickerProps {
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

const availabilityLabel = (a: string) => {
	switch (a) {
		case "force_on":
			return "Always on";
		case "default_on":
			return "On by default";
		case "default_off":
			return "Optional";
		default:
			return a;
	}
};

const MCPIcon: FC<{ iconUrl: string; name: string; className?: string }> = ({
	iconUrl,
	name,
	className,
}) => {
	const icon = iconUrl ? (
		<ExternalImage src={iconUrl} alt={`${name} icon`} className="size-3/5" />
	) : (
		<ServerIcon className="size-3/5 text-content-secondary" />
	);

	return (
		<div
			className={cn(
				"flex shrink-0 items-center justify-center rounded-full bg-surface-secondary",
				className,
			)}
		>
			{icon}
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
	const ids: string[] = [];
	for (const server of servers) {
		if (
			server.enabled &&
			(server.availability === "force_on" ||
				server.availability === "default_on")
		) {
			ids.push(server.id);
		}
	}
	return ids;
};

/**
 * Returns true when the server cannot be used yet because the calling
 * user still has authentication work to do: OAuth2 sign-in for
 * `oauth2` servers, or user-supplied header values for
 * `custom_headers` servers with admin-marked user keys. Used both to
 * decide which control to render in the picker and to exclude
 * unconnected servers from the trigger icon stack.
 */
export const mcpServerNeedsAuth = (server: TypesGen.MCPServerConfig): boolean =>
	(server.auth_type === "oauth2" || server.auth_type === "custom_headers") &&
	!server.auth_connected;

/** Route that hosts the user MCP settings page with the Configure dialog. */
export const userMCPServersSettingsPath = "/agents/settings/user-mcp-servers";

/** localStorage key for persisting the user's MCP server selection. */
export const mcpSelectionStorageKey = "agents.selected-mcp-server-ids";

/**
 * Read the persisted MCP selection from localStorage, filtered to only
 * include IDs that still exist in the current server list.
 * Returns `null` when nothing is stored (caller should fall back to defaults).
 */ export const getSavedMCPSelection = (
	servers: readonly TypesGen.MCPServerConfig[],
): string[] | null => {
	const raw = localStorage.getItem(mcpSelectionStorageKey);
	if (raw === null) {
		return null;
	}
	// If the server list is empty (e.g. the query hasn't loaded yet),
	// we can't validate any IDs so signal "unknown" rather than
	// returning an empty array that would be mistaken for "user
	// deliberately deselected everything".
	if (servers.length === 0) {
		return null;
	}
	try {
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) {
			return null;
		}
		const enabledIds = new Set<string>();
		const forceOnIds: string[] = [];
		for (const server of servers) {
			if (!server.enabled) continue;
			enabledIds.add(server.id);
			if (server.availability === "force_on") {
				forceOnIds.push(server.id);
			}
		}
		const restored = parsed.filter(
			(id): id is string => typeof id === "string" && enabledIds.has(id),
		);
		// Merge force_on servers that might not be in the saved list.
		for (const id of forceOnIds) {
			if (!restored.includes(id)) {
				restored.push(id);
			}
		}
		return restored;
	} catch {
		return null;
	}
};

/**
 * Persist the current MCP selection to localStorage.
 */ export const saveMCPSelection = (ids: readonly string[]): void => {
	localStorage.setItem(mcpSelectionStorageKey, JSON.stringify(ids));
};

// ── Overlapping icon stack for the trigger ─────────────────────

const ICON_STACK_MAX = 3;

const TriggerIconStack: FC<{
	servers: readonly TypesGen.MCPServerConfig[];
}> = ({ servers }) => {
	const visible = servers.slice(0, ICON_STACK_MAX);
	return (
		<span className="inline-flex items-center">
			{visible.map((s, i) => (
				<span
					key={s.id}
					className={cn(
						"inline-flex rounded-full ring-1 ring-surface-primary",
						i > 0 && "-ml-1.5",
					)}
				>
					<MCPIcon
						iconUrl={s.icon_url}
						name={s.display_name}
						className="size-4"
					/>
				</span>
			))}
			{servers.length > ICON_STACK_MAX && (
				<span className="-ml-1 inline-flex size-4 items-center justify-center rounded-full bg-surface-secondary text-[9px] font-medium text-content-secondary ring-1 ring-surface-primary">
					+{servers.length - ICON_STACK_MAX}
				</span>
			)}
		</span>
	);
};

// ── Per-server auth/toggle control ─────────────────────────────

interface MCPServerAuthControlProps {
	server: TypesGen.MCPServerConfig;
	isSelected: boolean;
	isConnecting: boolean;
	disabled: boolean;
	/** Disable the OAuth2 Auth button while another connect flow is in progress. */
	connectingDisabled: boolean;
	forceOn: boolean;
	onConnect: (server: TypesGen.MCPServerConfig) => void;
	onConfigure: () => void;
	onToggle: (id: string, checked: boolean) => void;
	/** Extra classes for the Auth/Configure button. */
	buttonClassName?: string;
	switchSize?: "sm";
}

/**
 * Renders the right-hand control for a single MCP server row:
 * - an `Auth` button when an OAuth2 server still needs sign-in,
 * - a `Configure` button when a `custom_headers` server still needs
 *   per-user header values supplied,
 * - otherwise a Switch toggle for inclusion in the conversation.
 *
 * Shared between `MCPServerPicker` (used in `AgentCreateForm`) and
 * the inline MCP picker inside `AgentChatInput`'s plus menu.
 */
export const MCPServerAuthControl: FC<MCPServerAuthControlProps> = ({
	server,
	isSelected,
	isConnecting,
	disabled,
	connectingDisabled,
	forceOn,
	onConnect,
	onConfigure,
	onToggle,
	buttonClassName,
	switchSize,
}) => {
	const needsOAuth = server.auth_type === "oauth2" && !server.auth_connected;
	const needsHeaderConfig =
		server.auth_type === "custom_headers" && !server.auth_connected;

	if (needsOAuth) {
		return (
			<Button
				variant="outline"
				size="sm"
				className={cn(
					"h-6 shrink-0 px-2 text-[10px] leading-none",
					buttonClassName,
				)}
				onClick={(e) => {
					e.stopPropagation();
					onConnect(server);
				}}
				disabled={disabled || connectingDisabled}
				aria-label={`Authenticate with ${server.display_name}`}
			>
				{isConnecting ? <Spinner loading className="h-2.5 w-2.5" /> : null}
				Auth
			</Button>
		);
	}

	if (needsHeaderConfig) {
		return (
			<Button
				variant="outline"
				size="sm"
				className={cn(
					"h-6 shrink-0 px-2 text-[10px] leading-none",
					buttonClassName,
				)}
				onClick={(e) => {
					e.stopPropagation();
					onConfigure();
				}}
				disabled={disabled}
				aria-label={`Configure ${server.display_name}`}
			>
				Configure
			</Button>
		);
	}

	return (
		<Switch
			size={switchSize}
			checked={isSelected}
			onCheckedChange={(checked) => onToggle(server.id, checked)}
			disabled={disabled || forceOn}
			aria-label={`${isSelected ? "Disable" : "Enable"} ${server.display_name}`}
		/>
	);
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

	// Servers shown in the trigger icon stack: selected and
	// fully ready (no outstanding auth required).
	const activeServers = enabledServers.filter(
		(s) =>
			(s.availability === "force_on" || selectedServerIds.includes(s.id)) &&
			!mcpServerNeedsAuth(s),
	);

	const navigate = useNavigate();

	// Listen for OAuth2 completion postMessage from popup.
	useEffect(() => {
		const handler = (event: MessageEvent) => {
			if (event.origin !== location.origin) return;
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

	// Poll for popup close and clean up on unmount.
	useEffect(() => {
		if (!connectingServerId || !popupRef.current) return;
		const interval = setInterval(() => {
			if (popupRef.current?.closed) {
				setConnectingServerId(null);
				popupRef.current = null;
			}
		}, 500);
		return () => {
			clearInterval(interval);
			// Close the popup if the component unmounts while
			// an auth flow is still in progress.
			if (popupRef.current && !popupRef.current.closed) {
				popupRef.current.close();
				popupRef.current = null;
			}
		};
	}, [connectingServerId]);

	const handleToggle = (serverId: string, checked: boolean) => {
		if (checked) {
			onSelectionChange([...selectedServerIds, serverId]);
		} else {
			onSelectionChange(selectedServerIds.filter((id) => id !== serverId));
		}
	};

	const handleConnect = (server: TypesGen.MCPServerConfig) => {
		setConnectingServerId(server.id);
		const connectUrl = `/api/experimental/mcp/servers/${encodeURIComponent(server.id)}/oauth2/connect`;
		popupRef.current = window.open(
			connectUrl,
			"_blank",
			"width=900,height=600",
		);
	};

	const handleConfigureHeaders = () => {
		setOpen(false);
		navigate(userMCPServersSettingsPath);
	};

	if (enabledServers.length === 0) {
		return null;
	}

	return (
		<Popover open={open} onOpenChange={setOpen}>
			<PopoverTrigger asChild>
				<button
					type="button"
					disabled={disabled}
					aria-label="MCP Servers"
					className="group flex h-8 w-full cursor-pointer items-center gap-1.5 border-none bg-transparent px-1 text-xs text-content-secondary shadow-none transition-colors hover:text-content-primary disabled:cursor-not-allowed disabled:opacity-50"
				>
					<span>MCP</span>
					{activeServers.length > 0 && (
						<TriggerIconStack servers={activeServers} />
					)}
					<ChevronDownIcon className="ml-auto size-3.5 text-content-secondary transition-colors group-hover:text-content-primary" />
				</button>
			</PopoverTrigger>
			<PopoverContent align="start" className="w-52 p-0">
				<TooltipProvider delayDuration={300}>
					<div className="max-h-64 overflow-y-auto py-1 [scrollbar-width:thin]">
						{enabledServers.map((server) => {
							const isForceOn = server.availability === "force_on";
							const isSelected =
								isForceOn || selectedServerIds.includes(server.id);
							const isConnecting = connectingServerId === server.id;

							return (
								<Tooltip key={server.id}>
									<TooltipTrigger asChild>
										<div className="flex items-center gap-2 px-2.5 py-1.5">
											<MCPIcon
												iconUrl={server.icon_url}
												name={server.display_name}
												className="size-5"
											/>
											<span className="min-w-0 flex-1 truncate text-xs text-content-primary">
												{server.display_name}
											</span>
											{isForceOn && (
												<LockIcon className="size-3 shrink-0 text-content-secondary" />
											)}
											<MCPServerAuthControl
												server={server}
												isSelected={isSelected}
												isConnecting={isConnecting}
												disabled={disabled}
												connectingDisabled={connectingServerId !== null}
												forceOn={isForceOn}
												onConnect={handleConnect}
												onConfigure={handleConfigureHeaders}
												onToggle={handleToggle}
												buttonClassName="w-fit min-w-0 gap-0 border-border/50"
											/>
										</div>
									</TooltipTrigger>
									<TooltipContent
										side="right"
										sideOffset={8}
										className="max-w-[220px] px-2.5 py-1.5"
									>
										<span className="block font-semibold leading-tight text-content-primary">
											{server.display_name}
										</span>
										{server.description && (
											<span className="block leading-tight text-content-secondary">
												{server.description}
											</span>
										)}
										<span className="mt-1 block text-content-secondary leading-tight">
											{availabilityLabel(server.availability)}
										</span>
										{server.auth_type !== "none" && (
											<span className="block text-content-secondary leading-tight">
												{server.auth_connected
													? "Authenticated"
													: "Not authenticated"}
											</span>
										)}
									</TooltipContent>
								</Tooltip>
							);
						})}
					</div>
				</TooltipProvider>
			</PopoverContent>
		</Popover>
	);
};
