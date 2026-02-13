import Collapse from "@mui/material/Collapse";
import type { WorkspaceConnectionStatus } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import {
	StatusIndicator,
	StatusIndicatorDot,
} from "components/StatusIndicator/StatusIndicator";
import { TableCell } from "components/Table/Table";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { InfoIcon } from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";
import { ForensicTimeline } from "./ForensicTimeline";
import type { DiagnosticSession, DiagnosticSessionConnection } from "./types";

interface SessionRowProps {
	session: DiagnosticSession;
}

const baseStatusVariant: Record<
	WorkspaceConnectionStatus,
	"success" | "warning" | "inactive" | "failed"
> = {
	ongoing: "success",
	control_lost: "warning",
	clean_disconnected: "inactive",
	client_disconnected: "inactive",
};

const baseStatusLabel: Record<WorkspaceConnectionStatus, string> = {
	ongoing: "Connected",
	control_lost: "Control Lost",
	clean_disconnected: "Disconnected",
	client_disconnected: "Client Disconnected",
};

type StatusVariant = "success" | "warning" | "inactive" | "failed";

function getStatusVariant(session: DiagnosticSession): StatusVariant {
	const reason = session.disconnect_reason.toLowerCase();
	if (
		reason.includes("workspace stopped") ||
		reason.includes("workspace deleted")
	) {
		return "warning";
	}
	if (reason.includes("agent timeout")) {
		return "failed";
	}
	return baseStatusVariant[session.status];
}

function getDisplayLabel(session: DiagnosticSession): string {
	const reason = session.disconnect_reason.toLowerCase();
	if (reason.includes("workspace stopped")) return "Workspace Stopped";
	if (reason.includes("workspace deleted")) return "Workspace Deleted";
	if (reason.includes("agent timeout")) return "Agent Timeout";
	return baseStatusLabel[session.status];
}

function formatDuration(seconds: number | null): string {
	if (seconds === null) return "ongoing";
	if (seconds < 60) return `${seconds}s`;
	const m = Math.floor(seconds / 60);
	const h = Math.floor(m / 60);
	if (h > 0) return `${h}h ${m % 60}m`;
	return `${m}m ${seconds % 60}s`;
}

function formatTimeShort(iso: string): string {
	return new Date(iso).toLocaleTimeString([], {
		hour: "2-digit",
		minute: "2-digit",
		hour12: false,
	});
}

const friendlyType: Record<string, string> = {
	vscode: "VS Code",
	ssh: "SSH",
	reconnecting_pty: "Terminal",
	workspace_app: "App",
	port_forwarding: "Port",
	jetbrains: "JetBrains",
	system: "System",
};

function connLabel(conn: DiagnosticSessionConnection): string {
	if (conn.type === "workspace_app" && conn.detail) return conn.detail;
	if (conn.type === "port_forwarding" && conn.detail) return `Port ${conn.detail}`;
	return friendlyType[conn.type] || conn.type;
}

// typeDisplayLabel is the PROMINENT first column. Shows what kind of
// activity this session represents: "VS Code", "SSH", "code-server (2)",
// "code-server (2), Terminal", etc.
function typeDisplayLabel(session: DiagnosticSession): string {
	if (session.connections.length === 0) return "";

	// Group by display label, skipping system connections so they
	// don't dominate the title (system wraps the real user connection).
	const groups = new Map<string, number>();
	for (const conn of session.connections) {
		if (conn.type === "system") continue;
		const lbl = connLabel(conn);
		groups.set(lbl, (groups.get(lbl) ?? 0) + 1);
	}

	// Fall back to "System" when every connection is system.
	if (groups.size === 0) {
		const count = session.connections.length;
		return count > 1 ? `System (${count})` : "System";
	}

	const parts: string[] = [];
	for (const [lbl, count] of groups) {
		parts.push(count > 1 ? `${lbl} (${count})` : lbl);
	}
	return parts.join(", ");
}

// clientDisplayLabel is the primary line in the client identity column.
function clientDisplayLabel(session: DiagnosticSession): string {
	if (session.short_description) return session.short_description;
	if (session.client_hostname) return session.client_hostname;
	if (session.ip === "127.0.0.1") return "127.0.0.1 (local)";
	return session.ip || "Unknown";
}

// clientSecondaryLabel is the second line (hostname) when the primary
// line is a description. Returns null when there's nothing extra to show.
function clientSecondaryLabel(session: DiagnosticSession): string | null {
	if (session.short_description && session.client_hostname) {
		return session.client_hostname;
	}
	return null;
}

// tooltipIP returns the IP for an (i) tooltip. Tailnet IPs are shown
// via tooltip when a friendlier label exists.
function tooltipIP(session: DiagnosticSession): string | null {
	const hasLabel = session.short_description || session.client_hostname;
	if (!hasLabel || !session.ip) return null;
	return session.ip;
}

const ConnectionSubRow: FC<{ conn: DiagnosticSessionConnection }> = ({
	conn,
}) => {
	const isSystem = conn.type === "system";
	return (
		<div className={cn(
			"flex items-center gap-3 py-1.5 px-3 text-xs border-t border-border",
			isSystem && "opacity-50",
		)}>
			<StatusIndicatorDot variant={baseStatusVariant[conn.status]} size="sm" />
			<Badge size="xs">{conn.type}</Badge>
			<span className="text-content-secondary truncate">{conn.detail}</span>
			<span className="ml-auto text-content-secondary font-mono text-2xs">
				{formatTimeShort(conn.connected_at)}
				{conn.disconnected_at && ` → ${formatTimeShort(conn.disconnected_at)}`}
			</span>
			{conn.exit_code !== null && (
				<span className="text-2xs text-content-secondary">
					exit {conn.exit_code}
				</span>
			)}
		</div>
	);
};

export const SessionRow: FC<SessionRowProps> = ({ session }) => {
	const [open, setOpen] = useState(false);
	const variant = getStatusVariant(session);
	const label = getDisplayLabel(session);
	const typeLabel = typeDisplayLabel(session);
	const clientLabel = clientDisplayLabel(session);
	const clientSecondary = clientSecondaryLabel(session);
	const ipForTooltip = tooltipIP(session);

	const toggle = () => setOpen((v) => !v);

	return (
		<TimelineEntry
			data-testid={`session-row-${session.id}`}
			clickable
			className="[&_td]:before:hidden"
		>
			<TableCell css={{ padding: "0 !important", border: 0 }}>
				<div
					className="flex items-center gap-3 px-8 py-3 cursor-pointer"
					onClick={toggle}
					onKeyDown={(e) => {
						if (e.key === "Enter") toggle();
					}}
					tabIndex={0}
					role="button"
				>
					<StatusIndicatorDot variant={variant} size="sm" />

					{/* Type + detail: PROMINENT first column */}
					<span className="text-sm font-medium text-content-primary w-36 shrink-0 truncate">
						{typeLabel}
					</span>

					{/* Source / client description */}
					<span className="text-xs text-content-secondary truncate min-w-0 flex-1 inline-flex items-center gap-1.5">
						{clientLabel}
						{ipForTooltip && (
							<Tooltip>
								<TooltipTrigger asChild>
									<InfoIcon className="size-3.5 text-content-secondary shrink-0" />
								</TooltipTrigger>
								<TooltipContent>{ipForTooltip}</TooltipContent>
							</Tooltip>
						)}
					</span>

					{/* Hostname */}
					<span className="text-xs font-mono text-content-secondary w-24 shrink-0 truncate">
						{clientSecondary || ""}
					</span>

					{/* Workspace */}
					<span className="text-xs text-content-secondary w-36 shrink-0 truncate">
						{session.workspace_name}
					</span>

					{/* Duration */}
					<span className="text-xs text-content-secondary w-20 shrink-0 text-right">
						{formatDuration(session.duration_seconds)}
					</span>

					{/* Time */}
					<span className="text-2xs font-mono text-content-secondary w-12 shrink-0 text-right">
						{formatTimeShort(session.started_at)}
					</span>

					{/* Status */}
					<StatusIndicator
						variant={variant}
						size="sm"
						className="w-36 shrink-0 justify-end"
					>
						<StatusIndicatorDot />
						{label}
					</StatusIndicator>

					<DropdownArrow close={open} margin={false} />
				</div>

				<Collapse in={open}>
					<div className="bg-surface-secondary border-t border-border px-8 py-4">
						{/* Explanation */}
						{session.explanation && (
							<p className="text-xs italic text-content-secondary mb-3">
								{session.explanation}
							</p>
						)}

						{/* Network summary */}
						{(session.network.p2p !== null ||
							session.network.avg_latency_ms !== null ||
							session.network.home_derp !== null) && (
							<div className="flex items-center gap-4 text-xs mb-3">
								<span className="text-content-secondary">Network:</span>
								{session.network.p2p !== null && (
									<Badge
										size="xs"
										variant={session.network.p2p ? "green" : "info"}
									>
										{session.network.p2p ? "P2P" : "DERP"}
									</Badge>
								)}
								{session.network.avg_latency_ms !== null && (
									<span className="text-content-secondary">
										{session.network.avg_latency_ms.toFixed(0)}ms avg
									</span>
								)}
								{session.network.home_derp && (
									<span className="text-content-secondary">
										{session.network.home_derp}
									</span>
								)}
								<span className="ml-auto text-content-secondary font-mono text-2xs">
									{formatTimeShort(session.started_at)}
									{session.ended_at &&
										` → ${formatTimeShort(session.ended_at)}`}
								</span>
							</div>
						)}

						{/* Connections list */}
						{session.connections.length > 0 && (
							<div
								className={
									session.network.p2p !== null ||
									session.network.avg_latency_ms !== null ||
									session.network.home_derp !== null
										? "pt-3 border-t border-border mb-3"
										: "mb-3"
								}
							>
								<h4 className="text-xs font-medium text-content-secondary mb-2">
									Connections
								</h4>
								{session.connections.map((conn) => (
									<ConnectionSubRow key={conn.id} conn={conn} />
								))}
							</div>
						)}

						{/* Timeline */}
						{session.timeline.length > 0 && (
							<div className="pt-3 border-t border-border">
								<h4 className="text-xs font-medium text-content-secondary mb-2">
									Timeline
								</h4>
								<ForensicTimeline events={session.timeline} />
							</div>
						)}
					</div>
				</Collapse>
			</TableCell>
		</TimelineEntry>
	);
};
