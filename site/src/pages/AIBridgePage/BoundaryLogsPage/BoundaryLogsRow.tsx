import type { BoundaryAuditLog } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { TableCell, TableRow } from "components/Table/Table";
import { TimelineEntry } from "components/Timeline/TimelineEntry";
import type { FC } from "react";

interface BoundaryLogsRowProps {
	log: BoundaryAuditLog;
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
				<TableCell>{log.resource_type}</TableCell>
				<TableCell className="font-mono text-sm">{log.resource}</TableCell>
				<TableCell>{log.operation}</TableCell>
				<TableCell>
					<Badge variant={log.decision === "allow" ? "green" : "destructive"}>
						{log.decision}
					</Badge>
				</TableCell>
			</TableRow>
		</TimelineEntry>
	);
};
