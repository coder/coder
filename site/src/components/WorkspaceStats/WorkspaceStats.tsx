import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import { OutdatedHelpTooltip } from "components/Tooltips"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { combineClasses } from "util/combineClasses"
import { createDayString } from "util/createDayString"
import {
  getDisplayWorkspaceBuildInitiatedBy,
  getDisplayWorkspaceTemplateName,
} from "util/workspace"
import { Workspace } from "../../api/typesGenerated"

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
  const styles = useStyles()
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(
    workspace.latest_build,
  )
  const displayTemplateName = getDisplayWorkspaceTemplateName(workspace)

  return (
    <div className={styles.stats} aria-label={Language.workspaceDetails}>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.templateLabel}:</span>
        <Link
          component={RouterLink}
          to={`/templates/${workspace.template_name}`}
          className={combineClasses([styles.statsValue, styles.link])}
        >
          {displayTemplateName}
        </Link>
      </div>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.versionLabel}:</span>
        <span className={styles.statsValue}>
          {workspace.outdated ? (
            <span className={styles.outdatedLabel}>
              {Language.outdated}
              <OutdatedHelpTooltip
                onUpdateVersion={handleUpdate}
                ariaLabel="update version"
              />
            </span>
          ) : (
            Language.upToDate
          )}
        </span>
      </div>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.lastBuiltLabel}:</span>
        <span className={styles.statsValue} data-chromatic="ignore">
          {createDayString(workspace.latest_build.created_at)}
        </span>
      </div>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.byLabel}:</span>
        <span className={styles.statsValue}>{initiatedBy}</span>
      </div>
      {workspace.latest_build.daily_cost > 0 && (
        <div className={styles.statItem}>
          <span className={styles.statsLabel}>{Language.costLabel}:</span>
          <span className={styles.statsValue}>
            {workspace.latest_build.daily_cost} / {quota_budget}
          </span>
        </div>
      )}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  stats: {
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    margin: "0px",
    [theme.breakpoints.down("sm")]: {
      display: "block",
    },
  },

  statItem: {
    padding: theme.spacing(2),
    paddingTop: theme.spacing(1.75),
    display: "flex",
    alignItems: "baseline",
    gap: theme.spacing(1),
  },

  statsLabel: {
    display: "block",
    wordWrap: "break-word",
  },

  statsValue: {
    marginTop: theme.spacing(0.25),
    display: "block",
    wordWrap: "break-word",
    color: theme.palette.text.primary,
  },

  capitalize: {
    textTransform: "capitalize",
  },

  link: {
    color: theme.palette.text.primary,
    fontWeight: 600,
  },

  outdatedLabel: {
    color: theme.palette.error.main,
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(0.5),
  },
}))
