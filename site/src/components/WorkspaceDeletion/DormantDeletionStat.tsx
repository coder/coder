import { StatsItem } from "components/Stats/Stats";
import Link from "@mui/material/Link";
import { type FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import type { Workspace } from "api/typesGenerated";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { displayDormantDeletion } from "./utils";

interface DormantDeletionStatProps {
  workspace: Workspace;
}

export const DormantDeletionStat: FC<DormantDeletionStatProps> = ({
  workspace,
}) => {
  const { entitlements, experiments } = useDashboard();
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions");

  if (
    !displayDormantDeletion(
      workspace,
      allowAdvancedScheduling,
      allowWorkspaceActions,
    )
  ) {
    return null;
  }

  return (
    <StatsItem
      label="Deletion on"
      className="containerClass"
      value={
        <Link
          component={RouterLink}
          to={`/templates/${workspace.template_name}/settings/schedule`}
          title="Schedule settings"
        >
          {/* We check for string existence in the conditional */}
          {new Date(workspace.deleting_at!).toLocaleString()}
        </Link>
      }
      css={{
        "&.containerClass": {
          flexDirection: "column",
          gap: 0,
          padding: 0,

          "& > span:first-of-type": {
            fontSize: 12,
            fontWeight: 500,
          },
        },
      }}
    />
  );
};
