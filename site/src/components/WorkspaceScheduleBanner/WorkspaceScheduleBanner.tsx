import Button from "@material-ui/core/Button"
import Alert from "@material-ui/lab/Alert"
import AlertTitle from "@material-ui/lab/AlertTitle"
import dayjs from "dayjs"
import isSameOrBefore from "dayjs/plugin/isSameOrBefore"
import utc from "dayjs/plugin/utc"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { isWorkspaceOn } from "../../util/workspace"

dayjs.extend(utc)
dayjs.extend(isSameOrBefore)

export const Language = {
  bannerAction: "Extend",
  bannerTitle: "Your workspace is scheduled to automatically shut down soon.",
}

export interface WorkspaceScheduleBannerProps {
  isLoading?: boolean
  onExtend: () => void
  workspace: TypesGen.Workspace
}

export const shouldDisplay = (workspace: TypesGen.Workspace): boolean => {
  if (!isWorkspaceOn(workspace)) {
    return false
  } else {
    // a manual shutdown has a deadline of '"0001-01-01T00:00:00Z"'
    // SEE: #1834
    const deadline = dayjs(workspace.latest_build.deadline).utc()
    const hasDeadline = deadline.year() > 1
    const thirtyMinutesFromNow = dayjs().add(30, "minutes").utc()
    return hasDeadline && deadline.isSameOrBefore(thirtyMinutesFromNow)
  }
}

export const WorkspaceScheduleBanner: FC<React.PropsWithChildren<WorkspaceScheduleBannerProps>> = ({
  isLoading,
  onExtend,
  workspace,
}) => {
  if (!shouldDisplay(workspace)) {
    return null
  } else {
    return (
      <Alert
        action={
          <Button color="inherit" disabled={isLoading} onClick={onExtend} size="small">
            {Language.bannerAction}
          </Button>
        }
        severity="warning"
      >
        <AlertTitle>{Language.bannerTitle}</AlertTitle>
      </Alert>
    )
  }
}
