import Link from "@material-ui/core/Link"
import { OutdatedHelpTooltip } from "components/Tooltips"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { createDayString } from "util/createDayString"
import {
  getDisplayWorkspaceBuildInitiatedBy,
  getDisplayWorkspaceTemplateName,
} from "util/workspace"
import { Workspace } from "../../api/typesGenerated"
import { Stats, StatsItem } from "components/Stats/Stats"

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
        value={createDayString(workspace.latest_build.created_at)}
      />
      <StatsItem label={Language.byLabel} value={initiatedBy} />
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
