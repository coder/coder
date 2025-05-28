import Tooltip from "@mui/material/Tooltip";
import type { Workspace } from "api/typesGenerated";
import { Badge } from "components/Badge/Badge";
import type { FC } from "react";
import {
	DATE_FORMAT,
	formatDateTime,
	relativeTimeWithoutSuffix,
} from "utils/time";

type WorkspaceDormantBadgeProps = {
	workspace: Workspace;
};

export const WorkspaceDormantBadge: FC<WorkspaceDormantBadgeProps> = ({
	workspace,
}) => {
	return workspace.deleting_at ? (
		<Tooltip
			title={
				<>
					This workspace has not been used for{" "}
					{relativeTimeWithoutSuffix(workspace.last_used_at)} and has been
					marked dormant. It is scheduled to be deleted on{" "}
					{formatDateTime(workspace.deleting_at, DATE_FORMAT.FULL_DATETIME)}.
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
					{relativeTimeWithoutSuffix(workspace.last_used_at)} and has been
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
