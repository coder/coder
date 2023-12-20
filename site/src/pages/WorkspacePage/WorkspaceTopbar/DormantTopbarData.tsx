import Link from "@mui/material/Link";
import { type FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import type { Workspace } from "api/typesGenerated";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { displayDormantDeletion } from "utils/dormant";
import { TopbarData, TopbarIcon } from "components/FullPageLayout/Topbar";
import DeleteOutline from "@mui/icons-material/DeleteOutline";

interface DormantTopbarDataProps {
  workspace: Workspace;
}

export const DormantTopbarData: FC<DormantTopbarDataProps> = ({
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
    <TopbarData>
      <TopbarIcon>
        <DeleteOutline />
      </TopbarIcon>
      <Link
        component={RouterLink}
        to={`/templates/${workspace.template_name}/settings/schedule`}
        title="Schedule settings"
        css={{ color: "inherit" }}
      >
        Deletion on {new Date(workspace.deleting_at!).toLocaleString()}
      </Link>
    </TopbarData>
  );
};
