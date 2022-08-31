import LinearProgress from '@material-ui/core/LinearProgress';
import { FC } from "react"
import { makeStyles } from "@material-ui/core/styles"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import Skeleton from '@material-ui/lab/Skeleton';
import { Stack } from 'components/Stack/Stack';
import Box from "@material-ui/core/Box";

export interface WorkspaceQuotaProps {
  loading: boolean
  count?: number
  limit?: number
}

export const WorkspaceQuota: FC<WorkspaceQuotaProps> = ({ loading, count, limit }) => {
  const styles = useStyles()

  const safeCount = count ? count : 0;
  const safeLimit = limit ? limit : 100;
  let value = Math.round((safeCount / safeLimit) * 100)
  // we don't want to round down to zero if the count is > 0
  if (safeCount > 0 && value === 0) {
    value = 1
  }
  const limitLanguage = limit ? limit : (<span className={styles.infinity}>∞</span>)

  return (
    <Box>
      <Stack spacing={1} className={styles.schedule}>
        {loading ? (
          <>
            <LinearProgress
              value={value}
              color="primary"
            />
            <div className={styles.scheduleLabel}>
              <Skeleton className={styles.skeleton}/>
            </div>
          </>
        ) : (
          <>
            <LinearProgress
                value={value}
                variant="determinate"
                color="primary"
            />
            <div className={styles.scheduleLabel}>
              {safeCount} of {limitLanguage} workspaces used
            </div>
          </>
        )}
      </Stack>
    </Box>
  )
}

const useStyles = makeStyles((theme) => ({
  schedule: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    display: 'inline-flex',
  },
  scheduleLabel: {
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
    color: theme.palette.text.secondary,
  },
  skeleton: {
    minWidth: "150px",
  },
  infinity: {
    fontSize: 18,
  },
}))
