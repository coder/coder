import Collapse from "@mui/material/Collapse";
import type {
	DiagnosticTimelineEvent,
	GlobalWorkspaceSession,
	UserDiagnosticResponse,
	WorkspaceConnection,
} from "api/typesGenerated";
import { API } from "api/api";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { TableCell } from "components/Table/Table";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import {
	connectionStatusColor,
	connectionStatusDot,
	connectionStatusLabel,
	connectionTypeLabel,
} from "modules/resources/ConnectionStatus";
import { ConnectionDetailDialog } from "pages/WorkspaceSessionsPage/ConnectionDetailDialog";
import { type FC, useState } from "react";
import { useQuery } from "react-query";
import { Link } from "react-router";

interface GlobalSessionRowProps {
	session: GlobalWorkspaceSession;
}

export const GlobalSessionRow: FC<GlobalSessionRowProps> = ({ session }) => {
	const [isOpen, setIsOpen] = useState(false);
	const [selectedConnection, setSelectedConnection] =
		useState<WorkspaceConnection | null>(null);

	const diagnosticQuery = useQuery<UserDiagnosticResponse>({
		queryKey: ["userDiagnostic", session.workspace_owner_username],
		queryFn: () => API.getUserDiagnostic(session.workspace_owner_username),
		enabled: selectedConnection !== null,
		staleTime: 60 * 1000,
	});

	const hasConnections = session.connections.length > 0;
	const selectedTimeline = findSessionTimeline(diagnosticQuery.data, session);

	// Client location: hostname or IP (where the connection came from).
	const clientLocation = session.client_hostname || session.ip || "Unknown";
	// Session description: what the session is (e.g. "Coder Server",
	// "CLI ssh"). Falls back to client location if not available.
	const sessionLabel = session.short_description || clientLocation;

	return (
		<>
			<TimelineEntry
				data-testid={`session-row-${session.id ?? session.started_at}`}
				clickable={hasConnections}
			>
				<TableCell className="py-4 pl-4 pr-8">
					{/* Summary row */}
					<div
						className="flex items-center gap-3 cursor-pointer"
						tabIndex={0}
						role="button"
						onClick={() => hasConnections && setIsOpen((v) => !v)}
						onKeyDown={(e) => {
							if (e.key === "Enter" && hasConnections) {
								setIsOpen((v) => !v);
							}
						}}
					>
						{/* Expand/collapse arrow — fixed width so content
						    aligns consistently whether arrow is shown or not. */}
						<span className="flex items-center justify-center w-6 shrink-0">
							{hasConnections && (
								<DropdownArrow close={isOpen} margin={false} />
							)}
						</span>

						{/* Workspace owner / name */}
						<div className="flex flex-col min-w-0 w-40 shrink-0">
							<Link
								to={`/connectionlog/diagnostics/${session.workspace_owner_username}`}
								className="text-sm font-medium text-content-primary truncate hover:underline"
								onClick={(e) => e.stopPropagation()}
							>
								{session.workspace_owner_username}
							</Link>
							<span className="text-xs text-content-secondary truncate">
								{session.workspace_name}
							</span>
						</div>

						{/* Session label and client location */}
						<div className="flex flex-col min-w-0 flex-1">
							<span className="text-sm text-content-primary truncate">
								{sessionLabel}
							</span>
							{session.short_description &&
								clientLocation !== session.short_description && (
									<span className="text-xs text-content-secondary font-mono truncate">
										{clientLocation}
									</span>
								)}
						</div>

						{/* Status */}
						<span className="flex items-center gap-1.5 shrink-0">
							<span
								className={`inline-block h-2 w-2 rounded-full ${connectionStatusDot(session.status)}`}
							/>
							<span
								className={`text-xs ${connectionStatusColor(session.status)}`}
							>
								{connectionStatusLabel(session.status)}
							</span>
						</span>

						{/* Connection count */}
						<span className="text-xs text-content-secondary shrink-0 w-24 text-right">
							{session.connections.length}{" "}
							{session.connections.length === 1 ? "connection" : "connections"}
						</span>

						{/* Time range */}
						<span className="text-xs text-content-secondary shrink-0 w-40 text-right">
							{formatTimeRange(session.started_at, session.ended_at)}
						</span>
					</div>

					{/* Expanded connections list */}
					<Collapse in={isOpen}>
						<div className="mt-3 ml-9 space-y-1">
							{session.connections.map((conn, idx) => (
								<button
									type="button"
									// Connections don't have guaranteed unique IDs, so we
									// use the index combined with created_at as a key.
									key={`${conn.created_at}-${idx}`}
									className="flex items-center gap-3 py-2 px-3 rounded cursor-pointer w-full text-left border-0 bg-transparent hover:bg-surface-secondary hover:outline focus:bg-surface-secondary focus:outline outline-1 -outline-offset-1 outline-border-hover transition-colors"
									onClick={(e) => {
										e.stopPropagation();
										setSelectedConnection(conn);
									}}
								>
									<span
										className={`inline-block h-2 w-2 rounded-full shrink-0 ${connectionStatusDot(conn.status)}`}
									/>
									<span className="text-sm font-medium text-content-primary">
										{connectionTypeLabel(conn.type, conn.detail)}
									</span>
									<span className="text-xs text-content-secondary">
										{conn.connected_at
											? new Date(conn.connected_at).toLocaleTimeString()
											: new Date(conn.created_at).toLocaleTimeString()}
									</span>
									<span
										className={`text-xs ${connectionStatusColor(conn.status)}`}
									>
										{connectionStatusLabel(conn.status)}
									</span>
									{conn.short_description && (
										<span className="text-xs text-content-secondary truncate">
											{conn.short_description}
										</span>
									)}
								</button>
							))}
						</div>
					</Collapse>
				</TableCell>
			</TimelineEntry>

			<ConnectionDetailDialog
				connection={selectedConnection}
				open={selectedConnection !== null}
				onClose={() => setSelectedConnection(null)}
				timeline={selectedTimeline}
				timelineLoading={diagnosticQuery.isLoading}
				timelineError={diagnosticQuery.error}
				showTimeline={selectedConnection !== null}
			/>
		</>
	);
};

function findSessionTimeline(
	diagnostic: UserDiagnosticResponse | undefined,
	session: GlobalWorkspaceSession,
): readonly DiagnosticTimelineEvent[] {
	if (!diagnostic) {
		return [];
	}

	const workspace =
		diagnostic.workspaces.find((w) => w.id === session.workspace_id) ??
		diagnostic.workspaces.find((w) => w.name === session.workspace_name);
	if (!workspace) {
		return [];
	}

	if (session.id) {
		const byID = workspace.sessions.find((s) => s.id === session.id);
		if (byID) {
			return byID.timeline;
		}
	}

	const byComposite = workspace.sessions.find(
		(s) =>
			s.started_at === session.started_at &&
			s.short_description === (session.short_description ?? "") &&
			s.client_hostname === (session.client_hostname ?? ""),
	);
	if (byComposite) {
		return byComposite.timeline;
	}

	const byStartTime = workspace.sessions.find(
		(s) => s.started_at === session.started_at,
	);
	return byStartTime?.timeline ?? [];
}

function formatTimeRange(startedAt: string, endedAt?: string): string {
	const start = new Date(startedAt);
	const startStr = start.toLocaleTimeString();
	if (!endedAt) {
		return `${startStr} — ongoing`;
	}
	const end = new Date(endedAt);
	return `${startStr} — ${end.toLocaleTimeString()}`;
}
