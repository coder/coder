import { Maybe } from "components/Conditionals/Maybe";
import { StatsItem } from "components/Stats/Stats";
import Link from "@mui/material/Link";
import { Link as RouterLink } from "react-router-dom";
import styled from "@emotion/styled";
import { Workspace } from "api/typesGenerated";
import { displayImpendingDeletion } from "./utils";
import { useDashboard } from "components/Dashboard/DashboardProvider";

export const ImpendingDeletionStat = ({
  workspace,
}: {
  workspace: Workspace;
}): JSX.Element => {
  const { entitlements, experiments } = useDashboard();
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions");

  return (
    <Maybe
      condition={displayImpendingDeletion(
        workspace,
        allowAdvancedScheduling,
        allowWorkspaceActions,
      )}
    >
      <StyledStatsItem
        label="Deletion on"
        className="containerClass"
        value={
          <Link
            component={RouterLink}
            to={`/templates/${workspace.template_name}/settings/schedule`}
            title="Schedule settings"
          >
            {/* We check for string existence in the conditional */}
            {new Date(workspace.deleting_at as string).toLocaleString()}
          </Link>
        }
      />
    </Maybe>
  );
};

const StyledStatsItem = styled(StatsItem)(() => ({
  "&.containerClass": {
    flexDirection: "column",
    gap: 0,
    padding: 0,

    "& > span:first-of-type": {
      fontSize: 12,
      fontWeight: 500,
    },
  },
}));
