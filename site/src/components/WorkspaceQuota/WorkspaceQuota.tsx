import Box from "@material-ui/core/Box"
import LinearProgress from "@material-ui/core/LinearProgress"
import { makeStyles } from "@material-ui/core/styles"
import Skeleton from "@material-ui/lab/Skeleton"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"

export const Language = {
  of: "of",
  workspace: "workspace",
  workspaces: "workspaces",
}

export interface WorkspaceQuotaProps {
  quota?: TypesGen.WorkspaceQuota
  error: Error | unknown
}

export const WorkspaceQuota: FC<WorkspaceQuotaProps> = ({ quota, error }) => {
  const styles = useStyles()

  // error state
  if (error !== undefined) {
    return (
      <Box>
        <Stack spacing={1} className={styles.stack}>
          <span className={styles.title}>Workspace Quota</span>
          <AlertBanner severity="error" error={error} />
        </Stack>
      </Box>
    )
  }

  // loading
  if (quota === undefined) {
    return (
      <Box>
        <Stack spacing={1} className={styles.stack}>
          <span className={styles.title}>Workspace Quota</span>
          <LinearProgress color="primary" />
          <div className={styles.label}>
            <Skeleton className={styles.skeleton} />
          </div>
        </Stack>
      </Box>
    )
  }

  // don't show if limit is 0, this means the feature is disabled.
  if (quota.user_workspace_limit === 0) {
    return null
  }

  let value = Math.round((quota.user_workspace_count / quota.user_workspace_limit) * 100)
  // we don't want to round down to zero if the count is > 0
  if (quota.user_workspace_count > 0 && value === 0) {
    value = 1
  }

  return (
    <Box>
      <Stack spacing={1} className={styles.stack}>
        <span className={styles.title}>Workspace Quota</span>
        <LinearProgress
          className={
            quota.user_workspace_count >= quota.user_workspace_limit
              ? styles.maxProgress
              : undefined
          }
          value={value}
          variant="determinate"
        />
        <div className={styles.label}>
          {quota.user_workspace_count} {Language.of} {quota.user_workspace_limit}{" "}
          {quota.user_workspace_limit === 1 ? Language.workspace : Language.workspaces}
          {" used"}
        </div>
      </Stack>
    </Box>
  )
}

const useStyles = makeStyles((theme) => ({
  stack: {
    paddingTop: theme.spacing(2.5),
  },
  maxProgress: {
    "& .MuiLinearProgress-colorPrimary": {
      backgroundColor: theme.palette.error.main,
    },
    "& .MuiLinearProgress-barColorPrimary": {
      backgroundColor: theme.palette.error.main,
    },
  },
  title: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 21,
    paddingBottom: "8px",
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
