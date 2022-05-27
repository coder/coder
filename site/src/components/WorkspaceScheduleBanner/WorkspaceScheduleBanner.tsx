import Alert from "@material-ui/lab/Alert"
import AlertTitle from "@material-ui/lab/AlertTitle"
import dayjs from "dayjs"
import isSameOrBefore from "dayjs/plugin/isSameOrBefore"
import utc from "dayjs/plugin/utc"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"

dayjs.extend(utc)
dayjs.extend(isSameOrBefore)

export const Language = {
  bannerTitle: "Workspace Shutdown",
  bannerDetail: "Your workspace will shutdown soon.",
}

export interface WorkspaceScheduleBannerProps {
  workspace: TypesGen.Workspace
}

export const shouldDisplay = (workspace: TypesGen.Workspace): boolean => {
  const transition = workspace.latest_build.transition
  const status = workspace.latest_build.job.status

  if (transition !== "start") {
    return false
  } else if (status === "canceled" || status === "canceling" || status === "failed") {
    return false
  } else {
    // a mannual shutdown has a deadline of '"0001-01-01T00:00:00Z"'
    // SEE: #1834
    const deadline = dayjs(workspace.latest_build.deadline).utc()
    const hasDeadline = deadline.year() > 1
    const thirtyMinutesFromNow = dayjs().add(30, "minutes").utc()
    return hasDeadline && deadline.isSameOrBefore(thirtyMinutesFromNow)
  }
}

export const WorkspaceScheduleBanner: React.FC<WorkspaceScheduleBannerProps> = ({ workspace }) => {
  if (!shouldDisplay(workspace)) {
    return null
  } else {
    return (
      <Alert severity="warning">
        <AlertTitle>{Language.bannerTitle}</AlertTitle>
        {Language.bannerDetail}
      </Alert>
    )
  }
}
