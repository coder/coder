import LinearProgress from '@material-ui/core/LinearProgress';
import { FC } from "react"
import { makeStyles } from "@material-ui/core/styles"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import Skeleton from '@material-ui/lab/Skeleton';
import { Stack } from 'components/Stack/Stack';
import Box from "@material-ui/core/Box";

export const Language = {
  of: "of",
  workspaceUsed: "workspace used",
  workspacesUsed: "workspaces used",
}

export interface WorkspaceQuotaProps {
  count?: number
  limit?: number
}

export const WorkspaceQuota: FC<WorkspaceQuotaProps> = ({ count, limit }) => {
  const styles = useStyles()

  // loading state
  if (count === undefined || limit === undefined) {
    return (
      <Box>
        <Stack spacing={1} className={styles.stack}>
          <LinearProgress
              color="primary"
          />
          <div className={styles.label}>
            <Skeleton className={styles.skeleton}/>
          </div>
        </Stack>
      </Box>
    )
  }

  let value = Math.round((count / limit) * 100)
  // we don't want to round down to zero if the count is > 0
  if (count > 0 && value === 0) {
    value = 1
  }

  return (
    <Box>
      <Stack spacing={1} className={styles.stack}>
        <LinearProgress
            value={value}
            variant="determinate"
            color="primary"
        />
        <div className={styles.label}>
          {count} {Language.of} {limit} {limit === 1 ? Language.workspaceUsed : Language.workspacesUsed }
        </div>
      </Stack>
    </Box>
  )
}

const useStyles = makeStyles((theme) => ({
  stack: {
    display: 'inline-flex',
  },
  label: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
    color: theme.palette.text.secondary,
  },
  skeleton: {
    minWidth: "150px",
  },
}))
