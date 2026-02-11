import Collapse from "@mui/material/Collapse";
import type {
	GlobalWorkspaceSession,
	WorkspaceConnection,
} from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { Stack } from "components/Stack/Stack";
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

interface GlobalSessionRowProps {
	session: GlobalWorkspaceSession;
}

export const GlobalSessionRow: FC<GlobalSessionRowProps> = ({ session }) => {
	const [isOpen, setIsOpen] = useState(false);
	const [selectedConnection, setSelectedConnection] =
		useState<WorkspaceConnection | null>(null);

	const hasConnections = session.connections.length > 0;

	return (
		<>
			<TimelineEntry
				data-testid={`session-row-${session.id ?? session.started_at}`}
				clickable={hasConnections}
			>
				<TableCell className="py-4 px-8">
					{/* Summary row */}
					<Stack
						direction="row"
						alignItems="center"
						className="cursor-pointer"
						tabIndex={0}
						onClick={() => hasConnections && setIsOpen((v) => !v)}
						onKeyDown={(e) => {
							if (e.key === "Enter" && hasConnections) {
								setIsOpen((v) => !v);
							}
						}}
					>
						{hasConnections && <DropdownArrow close={isOpen} />}
						<div className="flex items-center gap-3 flex-1 min-w-0">
							{/* Workspace info */}
							<div className="flex flex-col min-w-0">
								<span className="text-sm font-medium text-content-primary truncate">
									{session.workspace_name}
								</span>
								<span className="text-xs text-content-secondary truncate">
									{session.workspace_owner_username}
								</span>
							</div>

							{/* Client info */}
							<span className="text-xs text-content-secondary font-mono">
								{session.short_description ||
									session.client_hostname ||
									session.ip ||
									"Unknown"}
							</span>

							{/* Status */}
							<span className="flex items-center gap-1.5 ml-auto shrink-0">
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
							<span className="text-xs text-content-secondary shrink-0">
								{session.connections.length}{" "}
								{session.connections.length === 1
									? "connection"
									: "connections"}
							</span>

							{/* Time range */}
							<span className="text-xs text-content-secondary shrink-0">
								{formatTimeRange(session.started_at, session.ended_at)}
							</span>
						</div>
					</Stack>

					{/* Expanded connections list */}
					<Collapse in={isOpen}>
						<div className="mt-3 ml-8 space-y-1">
							{session.connections.map((conn, idx) => (
								<button
									type="button"
									// Connections don't have guaranteed unique IDs, so we
									// use the index combined with created_at as a key.
									key={`${conn.created_at}-${idx}`}
									className="flex items-center gap-3 py-2 px-3 rounded cursor-pointer hover:bg-surface-secondary w-full text-left"
									onClick={() => setSelectedConnection(conn)}
								>
									<span className="text-xs font-mono text-content-primary">
										{connectionTypeLabel(conn.type, conn.detail)}
									</span>
									<span className="text-xs text-content-secondary">
										{conn.connected_at
											? new Date(conn.connected_at).toLocaleTimeString()
											: new Date(conn.created_at).toLocaleTimeString()}
									</span>
									<span
										className={`inline-block h-2 w-2 rounded-full ${connectionStatusDot(conn.status)}`}
									/>
									<span
										className={`text-xs ${connectionStatusColor(conn.status)}`}
									>
										{connectionStatusLabel(conn.status)}
									</span>
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
			/>
		</>
	);
};

function formatTimeRange(startedAt: string, endedAt?: string): string {
	const start = new Date(startedAt);
	const startStr = start.toLocaleTimeString();
	if (!endedAt) {
		return `${startStr} — ongoing`;
	}
	const end = new Date(endedAt);
	return `${startStr} — ${end.toLocaleTimeString()}`;
}
