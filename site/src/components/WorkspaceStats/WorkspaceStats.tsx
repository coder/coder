import Link from "@material-ui/core/Link"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import { OutdatedHelpTooltip } from "components/Tooltips"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { combineClasses } from "util/combineClasses"
import { createDayString } from "util/createDayString"
import { getDisplayWorkspaceBuildInitiatedBy } from "util/workspace"
import { Workspace } from "../../api/typesGenerated"

const Language = {
  workspaceDetails: "Workspace Details",
  templateLabel: "Template",
  statusLabel: "Workspace Status",
  versionLabel: "Version",
  lastBuiltLabel: "Last Built",
  outdated: "Outdated",
  upToDate: "Up to date",
  byLabel: "Last built by",
}

export interface WorkspaceStatsProps {
  workspace: Workspace
  handleUpdate: () => void
}

export const WorkspaceStats: FC<WorkspaceStatsProps> = ({
  workspace,
  handleUpdate,
}) => {
  const styles = useStyles()
  const theme = useTheme()
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(
    workspace.latest_build,
  )

  return (
    <div className={styles.stats} aria-label={Language.workspaceDetails}>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.templateLabel}:</span>
        <Link
          component={RouterLink}
          to={`/templates/${workspace.template_name}`}
          className={combineClasses([styles.statsValue, styles.link])}
        >
          {workspace.template_name}
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
            <span style={{ color: theme.palette.text.secondary }}>
              {Language.upToDate}
            </span>
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
