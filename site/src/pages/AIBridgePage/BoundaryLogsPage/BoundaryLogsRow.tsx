import type { BoundaryNetworkAuditLog } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { TableCell, TableRow } from "components/Table/Table";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import type { FC } from "react";

interface BoundaryLogsRowProps {
	log: BoundaryNetworkAuditLog;
}

export const BoundaryLogsRow: FC<BoundaryLogsRowProps> = ({ log }) => {
	return (
		<TimelineEntry key={log.id} data-testid={`boundary-log-${log.id}`}>
			<TableRow>
				<TableCell>
					{new Date(log.time).toLocaleString()}
				</TableCell>
				<TableCell>{log.workspace_name}</TableCell>
				<TableCell>{log.workspace_owner_username}</TableCell>
				<TableCell>{log.agent_name}</TableCell>
				<TableCell className="font-mono text-sm">{log.domain}</TableCell>
				<TableCell>
					<Badge variant={log.action === "allow" ? "success" : "error"}>
						{log.action}
					</Badge>
				</TableCell>
			</TableRow>
		</TimelineEntry>
	);
};
