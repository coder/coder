import Link from "@material-ui/core/Link"
import { OutdatedHelpTooltip } from "components/Tooltips"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { createDayString } from "util/createDayString"
import {
  getDisplayWorkspaceBuildInitiatedBy,
  getDisplayWorkspaceTemplateName,
  isWorkspaceOn,
} from "util/workspace"
import { Workspace } from "../../api/typesGenerated"
import { Stats, StatsItem } from "components/Stats/Stats"
import upperFirst from "lodash/upperFirst"
import { autostopDisplay } from "util/schedule"

const Language = {
  workspaceDetails: "Workspace Details",
  templateLabel: "Template",
  statusLabel: "Workspace Status",
  versionLabel: "Version",
  lastBuiltLabel: "Last built",
  outdated: "Outdated",
  upToDate: "Up to date",
  byLabel: "Last built by",
  costLabel: "Daily cost",
}

export interface WorkspaceStatsProps {
  workspace: Workspace
  quota_budget?: number
  handleUpdate: () => void
}

export const WorkspaceStats: FC<WorkspaceStatsProps> = ({
  workspace,
  quota_budget,
  handleUpdate,
}) => {
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(
    workspace.latest_build,
  )
  const displayTemplateName = getDisplayWorkspaceTemplateName(workspace)

  return (
    <Stats aria-label={Language.workspaceDetails}>
      <StatsItem
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
        label={Language.versionLabel}
        value={
          <>
            <Link
              component={RouterLink}
              to={`/templates/${workspace.template_name}/versions/${workspace.latest_build.template_version_name}`}
            >
              {workspace.latest_build.template_version_name}
            </Link>

            {workspace.outdated && (
              <OutdatedHelpTooltip
                onUpdateVersion={handleUpdate}
                ariaLabel="update version"
              />
            )}
          </>
        }
      />
      <StatsItem
        label={Language.lastBuiltLabel}
        value={
          <>
            {upperFirst(createDayString(workspace.latest_build.created_at))} by{" "}
            {initiatedBy}
          </>
        }
      />
      {shouldDisplayScheduleLabel(workspace) && (
        <StatsItem
          label={getScheduleLabel(workspace)}
          value={autostopDisplay(workspace)}
        />
      )}
      {workspace.latest_build.daily_cost > 0 && (
        <StatsItem
          label={Language.costLabel}
          value={`${workspace.latest_build.daily_cost} ${
            quota_budget ? `/ ${quota_budget}` : ""
          }`}
        />
      )}
    </Stats>
  )
}

export const canEditDeadline = (workspace: Workspace): boolean => {
  return isWorkspaceOn(workspace) && Boolean(workspace.latest_build.deadline)
}

export const shouldDisplayScheduleLabel = (workspace: Workspace): boolean => {
  if (canEditDeadline(workspace)) {
    return true
  }
  if (isWorkspaceOn(workspace)) {
    return false
  }
  return Boolean(workspace.autostart_schedule)
}

const getScheduleLabel = (workspace: Workspace) => {
  return isWorkspaceOn(workspace) ? "Stops at" : "Starts at"
}
