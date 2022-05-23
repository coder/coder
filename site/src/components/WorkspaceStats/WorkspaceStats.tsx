import Link from "@material-ui/core/Link"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import dayjs from "dayjs"
import React from "react"
import { Link as RouterLink } from "react-router-dom"
import { Workspace } from "../../api/typesGenerated"
import { CardRadius, MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import { getDisplayStatus } from "../../util/workspace"

export interface WorkspaceStatsProps {
  workspace: Workspace
}

export const WorkspaceStats: React.FC<WorkspaceStatsProps> = ({ workspace }) => {
  const styles = useStyles()
  const theme = useTheme()
  const status = getDisplayStatus(theme, workspace.latest_build)

  return (
    <div className={styles.stats}>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Workspace</span>
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
        <span className={styles.statsLabel}>Status</span>
        <span className={styles.statsValue}>
          <span style={{ color: status.color }} role="status">
            {status.status}
          </span>
        </span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Version</span>
        <span className={styles.statsValue}>
          {workspace.outdated ? (
            <span style={{ color: theme.palette.error.main }}>outdated</span>
          ) : (
            <span style={{ color: theme.palette.text.secondary }}>up to date</span>
          )}
        </span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Last Built</span>
        <span className={styles.statsValue}>{dayjs().to(dayjs(workspace.latest_build.created_at))}</span>
      </div>
    </div>
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
    border: `1px solid ${theme.palette.divider}`,
  },

  statItem: {
    minWidth: theme.spacing(20),
    padding: theme.spacing(2),
    paddingTop: theme.spacing(1.75),
  },

  statsLabel: {
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
  },

  statsValue: {
    fontSize: 16,
    marginTop: theme.spacing(0.25),
    display: "inline-block",
  },

  statsDivider: {
    width: 1,
    height: theme.spacing(5),
    backgroundColor: theme.palette.divider,
    marginRight: theme.spacing(2),
  },

  capitalize: {
    textTransform: "capitalize",
  },

  link: {
    color: theme.palette.text.primary,
    fontWeight: 600,
  },
}))
