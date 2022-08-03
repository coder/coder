import Link from "@material-ui/core/Link"
import { makeStyles, useTheme } from "@material-ui/core/styles"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { WorkspaceBuild } from "../../api/typesGenerated"
import { CardRadius, MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { combineClasses } from "../../util/combineClasses"
import {
  displayWorkspaceBuildDuration,
  getDisplayWorkspaceBuildInitiatedBy,
  getDisplayWorkspaceBuildStatus,
} from "../../util/workspace"

export interface WorkspaceBuildStatsProps {
  build: WorkspaceBuild
}

export const WorkspaceBuildStats: FC<WorkspaceBuildStatsProps> = ({ build }) => {
  const styles = useStyles()
  const theme = useTheme()
  const status = getDisplayWorkspaceBuildStatus(theme, build)
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(build)

  return (
    <div className={styles.stats}>
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Workspace Name</span>
        <Link
          component={RouterLink}
          to={`/@${build.workspace_owner_name}/${build.workspace_name}`}
          className={combineClasses([styles.statsValue, styles.link])}
        >
          {build.workspace_name}
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
        <span className={combineClasses([styles.statsValue, styles.capitalize])}>
          {build.transition}
        </span>
      </div>
      <div className={styles.statsDivider} />
      <div className={styles.statItem}>
        <span className={styles.statsLabel}>Initiated by</span>
        <span className={styles.statsValue}>{initiatedBy}</span>
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
    [theme.breakpoints.down("sm")]: {
      display: "block",
    },
  },

  statItem: {
    minWidth: "13%",
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
