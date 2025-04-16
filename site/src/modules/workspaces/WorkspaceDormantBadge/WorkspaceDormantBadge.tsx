import Tooltip from "@mui/material/Tooltip";
import type { Workspace } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import { formatDistanceToNow } from "date-fns";
import type { FC } from "react";

export type WorkspaceDormantBadgeProps = {
	workspace: Workspace;
};

export const WorkspaceDormantBadge: FC<WorkspaceDormantBadgeProps> = ({
	workspace,
}) => {
	const formatDate = (dateStr: string): string => {
		const date = new Date(dateStr);
		return date.toLocaleDateString(undefined, {
			month: "long",
			day: "numeric",
			year: "numeric",
			hour: "numeric",
			minute: "numeric",
		});
	};

	return workspace.deleting_at ? (
		<Tooltip
			title={
				<>
					This workspace has not been used for{" "}
					{formatDistanceToNow(Date.parse(workspace.last_used_at))} and has been
					marked dormant. It is scheduled to be deleted on{" "}
					{formatDate(workspace.deleting_at)}.
				</>
			}
		>
			<Badge role="status" variant="destructive" size="xs">
				Deletion Pending
			</Badge>
		</Tooltip>
	) : (
		<Tooltip
			title={
				<>
					This workspace has not been used for{" "}
					{formatDistanceToNow(Date.parse(workspace.last_used_at))} and has been
					marked dormant. It is not scheduled for auto-deletion but will become
					a candidate if auto-deletion is enabled on this template.
				</>
			}
		>
			<Badge role="status" variant="warning" size="xs">
				Dormant
			</Badge>
		</Tooltip>
	);
};
