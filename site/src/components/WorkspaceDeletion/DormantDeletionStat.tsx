import { StatsItem } from "components/Stats/Stats";
import Link from "@mui/material/Link";
import { Link as RouterLink } from "react-router-dom";
import styled from "@emotion/styled";
import { Workspace } from "api/typesGenerated";
import { displayDormantDeletion } from "./utils";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { type FC } from "react";

interface DormantDeletionStatProps {
  workspace: Workspace;
}

export const DormantDeletionStat: FC<DormantDeletionStatProps> = ({
  workspace,
}) => {
  const { entitlements } = useDashboard();
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;

  if (!displayDormantDeletion(workspace, allowAdvancedScheduling)) {
    return null;
  }

  return (
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
          {new Date(workspace.deleting_at!).toLocaleString()}
        </Link>
      }
    />
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
