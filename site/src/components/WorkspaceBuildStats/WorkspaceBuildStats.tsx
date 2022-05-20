import Link from "@material-ui/core/Link"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import React from "react"
import { Link as RouterLink } from "react-router-dom"
import { WorkspaceBuild } from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import { displayWorkspaceBuildDuration, getDisplayStatus } from "../../util/workspace"

export interface WorkspaceBuildStatsProps {
  build: WorkspaceBuild
}

export const WorkspaceBuildStats: React.FC<WorkspaceBuildStatsProps> = ({ build }) => {
  const styles = useStyles()
  const theme = useTheme()
  const status = getDisplayStatus(theme, build)

  return (
    <div className={styles.stats}>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Workspace ID</span>
        <Link
          component={RouterLink}
          to={`/workspaces/${build.workspace_id}`}
          className={combineClasses([styles.statsValue, styles.link])}
        >
          {build.workspace_id}
        </Link>
      </div>
      <div className={styles.statsDivider} />

      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Duration</span>
        <span className={styles.statsValue}>{displayWorkspaceBuildDuration(build)}</span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Started at</span>
        <span className={styles.statsValue}>{new Date(build.created_at).toLocaleString()}</span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Status</span>
        <span className={styles.statsValue}>
          <span style={{ color: status.color }}>{status.status}</span>
        </span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Action</span>
        <span className={combineClasses([styles.statsValue, styles.capitalize])}>{build.transition}</span>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  stats: {
    paddingTop: theme.spacing(3),
    paddingBottom: theme.spacing(3),
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    fontFamily: MONOSPACE_FONT_FAMILY,
  },

  statItem: {
    minWidth: theme.spacing(20),
    paddingRight: theme.spacing(3),
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
    display: "block",
  },

  statsDivider: {
    width: 1,
    height: theme.spacing(5),
    backgroundColor: theme.palette.divider,
    marginRight: theme.spacing(3),
  },

  capitalize: {
    textTransform: "capitalize",
  },

  link: {
    color: theme.palette.text.primary,
    fontWeight: 600,
  },
}))
