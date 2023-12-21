import { type Interpolation, type Theme } from "@emotion/react";
import Link from "@mui/material/Link";
import { WorkspaceOutdatedTooltip } from "components/WorkspaceOutdatedTooltip/WorkspaceOutdatedTooltip";
import { type FC } from "react";
import { Link as RouterLink } from "react-router-dom";
import { getDisplayWorkspaceTemplateName } from "utils/workspace";
import type { Workspace } from "api/typesGenerated";
import { Stats, StatsItem } from "components/Stats/Stats";
import { WorkspaceStatusText } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge";
import { DormantDeletionStat } from "components/WorkspaceDeletion";
import { workspaceQuota } from "api/queries/workspaceQuota";
import { useQuery } from "react-query";
import _ from "lodash";
import {
  WorkspaceScheduleControls,
  scheduleLabel,
  shouldDisplayScheduleControls,
} from "./WorkspaceScheduleControls";

const Language = {
  workspaceDetails: "Workspace Details",
  templateLabel: "Template",
  costLabel: "Daily cost",
  updatePolicy: "Update policy",
};

export interface WorkspaceStatsProps {
  workspace: Workspace;
  canUpdateWorkspace: boolean;
  handleUpdate: () => void;
}

export const WorkspaceStats: FC<WorkspaceStatsProps> = ({
  workspace,
  canUpdateWorkspace,
  handleUpdate,
}) => {
  const displayTemplateName = getDisplayWorkspaceTemplateName(workspace);
  const quotaQuery = useQuery(workspaceQuota(workspace.owner_name));
  const quotaBudget = quotaQuery.data?.budget;

  return (
    <>
      <Stats aria-label={Language.workspaceDetails} css={styles.stats}>
        <StatsItem
          css={styles.statsItem}
          label="Status"
          value={<WorkspaceStatusText workspace={workspace} />}
        />
        <DormantDeletionStat workspace={workspace} />
        <StatsItem
          css={styles.statsItem}
          label={Language.templateLabel}
          value={
            <Link
              component={RouterLink}
              to={`/templates/${workspace.template_name}`}
            >
              {displayTemplateName}
            </Link>
          }
        />

        <StatsItem
          css={styles.statsItem}
          label="Version"
          value={
            <span css={{ display: "flex", alignItems: "center", gap: 4 }}>
              <Link
                component={RouterLink}
                to={`/templates/${workspace.template_name}/versions/${workspace.latest_build.template_version_name}`}
              >
                {workspace.latest_build.template_version_name}
              </Link>

              {workspace.outdated && (
                <WorkspaceOutdatedTooltip
                  templateName={workspace.template_name}
                  latestVersionId={workspace.template_active_version_id}
                  onUpdateVersion={handleUpdate}
                  ariaLabel="update version"
                />
              )}
            </span>
          }
        />

        {shouldDisplayScheduleControls(workspace) && (
          <StatsItem
            css={styles.statsItem}
            label={scheduleLabel(workspace)}
            value={
              <WorkspaceScheduleControls
                workspace={workspace}
                canUpdateSchedule={canUpdateWorkspace}
              />
            }
          />
        )}
        {workspace.latest_build.daily_cost > 0 && (
          <StatsItem
            css={styles.statsItem}
            label={Language.costLabel}
            value={`${workspace.latest_build.daily_cost} ${
              quotaBudget ? `/ ${quotaBudget}` : ""
            }`}
          />
        )}
      </Stats>
    </>
  );
};

const styles = {
  stats: (theme) => ({
    padding: 0,
    border: 0,
    gap: 48,
    rowGap: 24,
    flex: 1,

    [theme.breakpoints.down("md")]: {
      display: "flex",
      flexDirection: "column",
      alignItems: "flex-start",
      gap: 8,
    },
  }),

  statsItem: {
    flexDirection: "column",
    gap: 0,
    padding: 0,

    "& > span:first-of-type": {
      fontSize: 12,
      fontWeight: 500,
    },
  },
} satisfies Record<string, Interpolation<Theme>>;
