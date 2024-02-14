import { type FC } from "react";
import dayjs from "dayjs";
import relativeTime from "dayjs/plugin/relativeTime";
import Tooltip from "@mui/material/Tooltip";
import type { Workspace } from "api/typesGenerated";
import { useTime } from "hooks/useTime";
import type { WorkspaceActivityStatus } from "modules/workspaces/activity";
import { Pill } from "components/Pill/Pill";

dayjs.extend(relativeTime);

interface ActivityStatusProps {
  workspace: Workspace;
  status: WorkspaceActivityStatus;
}

export const ActivityStatus: FC<ActivityStatusProps> = ({
  workspace,
  status,
}) => {
  const usedAt = dayjs(workspace.last_used_at).tz(dayjs.tz.guess());

  // Don't bother updating if `status` will need to change before anything can happen.
  useTime(status === "ready" || status === "connected");

  switch (status) {
    case "ready":
      return <Pill type="active">Ready</Pill>;
    case "connected":
      return <Pill type="active">Connected</Pill>;
    case "inactive":
      return (
        <Tooltip
          title={
            <>
              This workspace was last active on{" "}
              {usedAt.format("MMMM D [at] h:mm A")}
            </>
          }
        >
          <Pill type="inactive">Inactive</Pill>
        </Tooltip>
      );
    case "notConnected":
      return (
        <Tooltip
          title={
            <>
              This workspace was last active on{" "}
              {usedAt.format("MMMM D [at] h:mm A")}
            </>
          }
        >
          <Pill type="inactive">Not connected</Pill>
        </Tooltip>
      );
  }

  return null;
};
