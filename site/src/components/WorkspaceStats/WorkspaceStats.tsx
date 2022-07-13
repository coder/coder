import Link from "@material-ui/core/Link"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import dayjs from "dayjs"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { Workspace } from "../../api/typesGenerated"
import { CardRadius, MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import { getDisplayStatus, getDisplayWorkspaceBuildInitiatedBy } from "../../util/workspace"
import { WorkspaceSection } from "../WorkspaceSection/WorkspaceSection"

const Language = {
  workspaceDetails: "Workspace Details",
  templateLabel: "Template",
  statusLabel: "Workspace Status",
  versionLabel: "Version",
  lastBuiltLabel: "Last Built",
  outdated: "Outdated",
  upToDate: "Up to date",
  byLabel: "Last Built by",
}

export interface WorkspaceStatsProps {
  workspace: Workspace
}

export const WorkspaceStats: FC<WorkspaceStatsProps> = ({ workspace }) => {
  const styles = useStyles()
  const theme = useTheme()
  const status = getDisplayStatus(theme, workspace.latest_build)
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(theme, workspace.latest_build)

  return (
    <WorkspaceSection title={Language.workspaceDetails} contentsProps={{ className: styles.stats }}>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.templateLabel}</span>
        <Link
          component={RouterLink}
          to={`/templates/${workspace.template_name}`}
          className={combineClasses([styles.statsValue, styles.link])}
        >
          {workspace.template_name}
        </Link>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.versionLabel}</span>
        <span className={styles.statsValue}>
          {workspace.outdated ? (
            <span style={{ color: theme.palette.error.main }}>{Language.outdated}</span>
          ) : (
            <span style={{ color: theme.palette.text.secondary }}>{Language.upToDate}</span>
          )}
        </span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.lastBuiltLabel}</span>
        <span className={styles.statsValue} data-chromatic="ignore">
          {dayjs().to(dayjs(workspace.latest_build.created_at))}
        </span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.byLabel}</span>
        <span className={styles.statsValue}>
          <span style={{ color: initiatedBy.color }}>{initiatedBy.initiatedBy}</span>
        </span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>{Language.statusLabel}</span>
        <span className={styles.statsValue}>
          <span style={{ color: status.color }} role="status">
            {status.status}
          </span>
        </span>
      </div>
    </WorkspaceSection>
  )
}

const useStyles = makeStyles((theme) => ({
  stats: {
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    backgroundColor: theme.palette.background.paper,
    borderRadius: CardRadius,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    margin: "0px",
    [theme.breakpoints.down("sm")]: {
      display: "block",
    },
  },

  statItem: {
    minWidth: "16%",
    padding: theme.spacing(2),
    paddingTop: theme.spacing(1.75),
  },

  statsLabel: {
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
    wordWrap: "break-word",
  },

  statsValue: {
    fontSize: 16,
    marginTop: theme.spacing(0.25),
    display: "block",
    wordWrap: "break-word",
  },

  statsDivider: {
    width: 1,
    height: theme.spacing(5),
    backgroundColor: theme.palette.divider,
    marginRight: theme.spacing(2),
    [theme.breakpoints.down("sm")]: {
      display: "none",
    },
  },

  capitalize: {
    textTransform: "capitalize",
  },

  link: {
    color: theme.palette.text.primary,
    fontWeight: 600,
  },
}))
