import Collapse from "@mui/material/Collapse";
import type {
	WorkspaceConnection,
	WorkspaceSession,
} from "api/typesGenerated";
import { DropdownArrow } from "components/DropdownArrow/DropdownArrow";
import { TableCell } from "components/Table/Table";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import {
	connectionStatusColor,
	connectionStatusDot,
	connectionStatusLabel,
	connectionTypeLabel,
} from "modules/resources/ConnectionStatus";
import { type FC, useState } from "react";
import { formatDateTime } from "utils/time";
import { ConnectionDetailDialog } from "./ConnectionDetailDialog";

interface WorkspaceSessionRowProps {
	session: WorkspaceSession;
}

export const WorkspaceSessionRow: FC<WorkspaceSessionRowProps> = ({
	session,
}) => {
	const [isOpen, setIsOpen] = useState(false);
	const [selectedConnection, setSelectedConnection] =
		useState<WorkspaceConnection | null>(null);
	const hasConnections = session.connections.length > 0;

	const clientLabel =
		session.client_hostname || session.ip || "Unknown client";
	const timeRange = session.ended_at
		? `${formatDateTime(session.started_at)} — ${formatDateTime(session.ended_at)}`
		: `${formatDateTime(session.started_at)} — ongoing`;

	return (
		<>
			<TimelineEntry
				clickable={hasConnections}
				onClick={() => hasConnections && setIsOpen((v) => !v)}
			>
				<TableCell>
					<div className="flex items-center gap-3 py-1">
						{hasConnections && (
							<DropdownArrow close={isOpen} />
						)}
						{!hasConnections && <div className="w-6" />}

						<div className="flex flex-col gap-1 flex-1 min-w-0">
							<div className="flex items-center gap-2">
								<span className="font-medium text-sm truncate">
									{clientLabel}
								</span>
								<span className="flex items-center gap-1.5">
									<span
										className={`inline-block h-2 w-2 rounded-full ${connectionStatusDot(session.status)}`}
									/>
									<span
										className={`text-xs ${connectionStatusColor(session.status)}`}
									>
										{connectionStatusLabel(session.status)}
									</span>
								</span>
								{hasConnections && (
									<span className="text-xs text-content-secondary">
										{session.connections.length}{" "}
										{session.connections.length === 1
											? "connection"
											: "connections"}
									</span>
								)}
							</div>
							<span className="text-xs text-content-secondary">
								{timeRange}
							</span>
						</div>
					</div>

					<Collapse in={isOpen}>
						<div className="mt-2 ml-9 mb-2 flex flex-col gap-0.5">
							{session.connections.map((conn, idx) => {
								const connLabel = connectionTypeLabel(
									conn.type,
									conn.detail,
								);
								const connTime = conn.connected_at ?? conn.created_at;
								return (
									<button
										type="button"
										// The combination of index and created_at
										// provides a stable-enough key for this list.
										key={`${conn.created_at}-${idx}`}
										className="flex items-center gap-3 py-2 px-3 rounded cursor-pointer hover:bg-surface-secondary text-left border-0 bg-transparent w-full"
										onClick={(e) => {
											e.stopPropagation();
											setSelectedConnection(conn);
										}}
									>
										<span
											className={`inline-block h-2 w-2 rounded-full shrink-0 ${connectionStatusDot(conn.status)}`}
										/>
										<span className="text-sm font-medium">
											{connLabel}
										</span>
										<span className="text-content-secondary text-xs">
											{formatDateTime(connTime)}
										</span>
										{conn.short_description && (
											<span className="text-content-secondary text-xs truncate">
												{conn.short_description}
											</span>
										)}
									</button>
								);
							})}
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
