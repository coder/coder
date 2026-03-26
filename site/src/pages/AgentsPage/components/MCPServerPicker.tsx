import { ChevronDownIcon, LockIcon, ServerIcon } from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import { cn } from "utils/cn";
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
		<ExternalImage src={iconUrl} alt={`${name} icon`} className="h-3/5 w-3/5" />
	) : (
		<ServerIcon className="h-3/5 w-3/5 text-content-secondary" />
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
	return servers
		.filter(
			(s) =>
				s.enabled &&
				(s.availability === "force_on" || s.availability === "default_on"),
		)
		.map((s) => s.id);
};

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
		const enabledIds = new Set(
			servers.filter((s) => s.enabled).map((s) => s.id),
		);
		// Always include force_on servers even if the user didn't save them.
		const forceOnIds = servers
			.filter((s) => s.enabled && s.availability === "force_on")
			.map((s) => s.id);
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
						className="h-4 w-4"
					/>
				</span>
			))}
			{servers.length > ICON_STACK_MAX && (
				<span className="-ml-1 inline-flex h-4 w-4 items-center justify-center rounded-full bg-surface-secondary text-[9px] font-medium text-content-secondary ring-1 ring-surface-primary">
					+{servers.length - ICON_STACK_MAX}
				</span>
			)}
		</span>
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
			!(s.auth_type === "oauth2" && !s.auth_connected),
	);

	// Listen for OAuth2 completion postMessage from popup.
	useEffect(() => {
		const handler = (event: MessageEvent) => {
			if (event.origin !== window.location.origin) return;
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
					<ChevronDownIcon className="ml-auto h-3.5 w-3.5 text-content-secondary transition-colors group-hover:text-content-primary" />
				</button>
			</PopoverTrigger>
			<PopoverContent align="start" className="w-52 p-0">
				<TooltipProvider delayDuration={300}>
					<div className="max-h-64 overflow-y-auto py-1 [scrollbar-width:thin]">
						{enabledServers.map((server) => {
							const isForceOn = server.availability === "force_on";
							const isSelected =
								isForceOn || selectedServerIds.includes(server.id);
							const needsAuth =
								server.auth_type === "oauth2" && !server.auth_connected;
							const isConnecting = connectingServerId === server.id;

							return (
								<Tooltip key={server.id}>
									<TooltipTrigger asChild>
										<div className="flex items-center gap-2 px-2.5 py-1.5">
											<MCPIcon
												iconUrl={server.icon_url}
												name={server.display_name}
												className="h-5 w-5"
											/>
											<span className="min-w-0 flex-1 truncate text-xs text-content-primary">
												{server.display_name}
											</span>
											{isForceOn && (
												<LockIcon className="h-3 w-3 shrink-0 text-content-secondary" />
											)}
											{needsAuth ? (
												<Button
													variant="outline"
													size="sm"
													className="h-6 w-fit min-w-0 shrink-0 gap-0 px-2 text-[10px] leading-none border-border/50"
													onClick={(e) => {
														e.stopPropagation();
														handleConnect(server);
													}}
													disabled={disabled || connectingServerId !== null}
													aria-label={`Authenticate with ${server.display_name}`}
												>
													{isConnecting ? (
														<Spinner loading className="h-2.5 w-2.5" />
													) : null}
													Auth
												</Button>
											) : (
												<Switch
													checked={isSelected}
													onCheckedChange={(checked) =>
														handleToggle(server.id, checked)
													}
													disabled={disabled || isForceOn}
													aria-label={`${isSelected ? "Disable" : "Enable"} ${server.display_name}`}
												/>
											)}
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
