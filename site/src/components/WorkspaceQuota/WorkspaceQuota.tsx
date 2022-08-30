import LinearProgress from '@material-ui/core/LinearProgress';
import { FC } from "react"
import { makeStyles } from "@material-ui/core/styles"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import Skeleton from '@material-ui/lab/Skeleton';

export interface WorkspaceQuotaProps {
  loading: boolean
  count?: number
  limit?: number
}

export const WorkspaceQuota: FC<WorkspaceQuotaProps> = ({ loading, count, limit }) => {
  const styles = useStyles()

  const safeCount = count ? count : 0;
  const safeLimit = limit ? limit : 100;
  const value = Math.round((safeCount / safeLimit) * 100)
  const limitLanguage = limit ? limit : `∞`

  return (
    <div className={styles.root}>
      <span>
        {loading ? (
            <div className={styles.item}>
              <div className={styles.quotaBar}>
                <LinearProgress
                  value={value}
                  color="primary"
                />
              </div>
              <div className={styles.quotaLabel}>
              <Skeleton />
              </div>
              <div className={styles.quotaLabel}>
                FILLER TEXT IDK HOW CSS WORKS
              </div>
            </div>
          ) : (
            <div className={styles.item}>
              <div className={styles.quotaBar}>
                <LinearProgress
                    value={value}
                    variant="determinate"
                    color="primary"
                />
              </div>
              <div className={styles.quotaLabel}>
                {safeCount} of {limitLanguage} workspaces used
              </div>
            </div>
          )}
      </span>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${theme.palette.divider}`,
    display: "flex",
    alignItems: "center",
    color: theme.palette.text.secondary,
    fontFamily: MONOSPACE_FONT_FAMILY,
    margin: "0px",
    [theme.breakpoints.down("sm")]: {
      display: "block",
    },
  },
  item: {
    minWidth: "16%",
    padding: theme.spacing(2),
  },
  quotaBar: {
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
    wordWrap: "break-word",
    paddingTop: theme.spacing(0.5),
  },
  quotaLabel: {
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
    wordWrap: "break-word",
    paddingTop: theme.spacing(0.5),
  },
}))
