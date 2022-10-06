import Button from "@material-ui/core/Button"
import dayjs from "dayjs"
import isSameOrBefore from "dayjs/plugin/isSameOrBefore"
import utc from "dayjs/plugin/utc"
import { FC } from "react"
import * as TypesGen from "api/typesGenerated"
import { isWorkspaceOn } from "util/workspace"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { useTranslation } from "react-i18next"

dayjs.extend(utc)
dayjs.extend(isSameOrBefore)

export interface WorkspaceScheduleBannerProps {
  isLoading?: boolean
  onExtend: () => void
  workspace: TypesGen.Workspace
}

export const shouldDisplay = (workspace: TypesGen.Workspace): boolean => {
  if (!isWorkspaceOn(workspace) || !workspace.latest_build.deadline) {
    return false
  }
  const deadline = dayjs(workspace.latest_build.deadline).utc()
  const thirtyMinutesFromNow = dayjs().add(30, "minutes").utc()
  return deadline.isSameOrBefore(thirtyMinutesFromNow)
}

export const WorkspaceScheduleBanner: FC<React.PropsWithChildren<WorkspaceScheduleBannerProps>> = ({
  isLoading,
  onExtend,
  workspace,
}) => {
  const { t } = useTranslation("workspacePage")

  if (!shouldDisplay(workspace)) {
    return null
  }

  const ScheduleButton = (
    <Button variant="outlined" disabled={isLoading} onClick={onExtend} size="small">
      {t("ctas.extendScheduleCta")}
    </Button>
  )

  return (
    <AlertBanner
      text={t("warningsAndErrors.workspaceShutdownWarning")}
      actions={[ScheduleButton]}
      severity="warning"
    />
  )
}
