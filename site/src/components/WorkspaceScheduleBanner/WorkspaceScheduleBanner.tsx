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
    if (!workspace.latest_build.deadline) {
      return false
    }
    const deadline = dayjs(workspace.latest_build.deadline).utc()
    const thirtyMinutesFromNow = dayjs().add(30, "minutes").utc()
    return deadline.isSameOrBefore(thirtyMinutesFromNow)
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
