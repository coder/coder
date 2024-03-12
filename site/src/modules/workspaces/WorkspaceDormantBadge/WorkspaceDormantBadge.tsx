import AutoDeleteIcon from "@mui/icons-material/AutoDelete";
import RecyclingIcon from "@mui/icons-material/Recycling";
import Tooltip from "@mui/material/Tooltip";
import { formatDistanceToNow } from "date-fns";
import type { FC } from "react";
import type { Workspace } from "api/typesGenerated";
import { Pill } from "components/Pill/Pill";

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
      <Pill role="status" icon={<AutoDeleteIcon />} type="error">
        Deletion Pending
      </Pill>
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
      <Pill role="status" icon={<RecyclingIcon />} type="warning">
        Dormant
      </Pill>
    </Tooltip>
  );
};
