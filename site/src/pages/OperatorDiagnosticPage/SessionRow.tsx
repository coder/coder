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
import { type FC, useState } from "react";
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

function connectionTypeLabel(session: DiagnosticSession): string {
	if (session.connections.length === 0) return "";
	return session.connections[0].type;
}

const ConnectionSubRow: FC<{ conn: DiagnosticSessionConnection }> = ({
	conn,
}) => (
	<div className="flex items-center gap-3 py-1.5 px-3 text-xs border-t border-border">
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

export const SessionRow: FC<SessionRowProps> = ({ session }) => {
	const [open, setOpen] = useState(false);
	const variant = getStatusVariant(session);
	const label = getDisplayLabel(session);
	const clientLabel =
		session.short_description || session.client_hostname || session.ip;
	const typeLabel = connectionTypeLabel(session);

	const toggle = () => setOpen((v) => !v);

	return (
		<TimelineEntry
			data-testid={`session-row-${session.id}`}
			clickable
			className="[&_td]:before:hidden"
		>
			<TableCell css={{ padding: "0 !important", border: 0 }}>
				<div
					className="flex items-center gap-3 px-8 py-4 cursor-pointer"
					onClick={toggle}
					onKeyDown={(e) => {
						if (e.key === "Enter") toggle();
					}}
					tabIndex={0}
					role="button"
				>
					<StatusIndicatorDot variant={variant} size="sm" />

					<span className="text-sm text-content-primary truncate min-w-0">
						{clientLabel}
					</span>

					<span className="text-xs text-content-secondary truncate">
						{session.workspace_name}
						{typeLabel && ` · ${typeLabel}`}
					</span>

					<span className="text-xs text-content-secondary ml-auto shrink-0">
						{formatDuration(session.duration_seconds)}
					</span>

					<span className="text-2xs font-mono text-content-secondary shrink-0">
						{formatTimeShort(session.started_at)}
					</span>

					<StatusIndicator variant={variant} size="sm">
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
