import { useMachine } from "@xstate/react"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { scheduleToAutoStart } from "pages/WorkspaceSchedulePage/schedule"
import { ttlMsToAutoStop } from "pages/WorkspaceSchedulePage/ttl"
import React, { useEffect } from "react"
import { useTranslation } from "react-i18next"
import { Navigate, useNavigate, useParams } from "react-router-dom"
import { scheduleChanged } from "util/schedule"
import * as TypesGen from "../../api/typesGenerated"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { WorkspaceScheduleForm } from "../../components/WorkspaceScheduleForm/WorkspaceScheduleForm"
import { firstOrItem } from "../../util/array"
import { workspaceSchedule } from "../../xServices/workspaceSchedule/workspaceScheduleXService"
import {
  formValuesToAutoStartRequest,
  formValuesToTTLRequest,
} from "./formToRequest"

const getAutoStart = (workspace?: TypesGen.Workspace) =>
  scheduleToAutoStart(workspace?.autostart_schedule)
const getAutoStop = (workspace?: TypesGen.Workspace) =>
  ttlMsToAutoStop(workspace?.ttl_ms)

export const WorkspaceSchedulePage: React.FC = () => {
  const { t } = useTranslation("workspaceSchedulePage")
  const { username: usernameQueryParam, workspace: workspaceQueryParam } =
    useParams()
  const navigate = useNavigate()
  const username = firstOrItem(usernameQueryParam, null)
  const workspaceName = firstOrItem(workspaceQueryParam, null)
  const [scheduleState, scheduleSend] = useMachine(workspaceSchedule)
  const {
    checkPermissionsError,
    submitScheduleError,
    getWorkspaceError,
    permissions,
    workspace,
    shouldRestartWorkspace,
  } = scheduleState.context

  // Get workspace on mount and whenever the args for getting a workspace change.
  // scheduleSend should not change.
  useEffect(() => {
    username &&
      workspaceName &&
      scheduleSend({ type: "GET_WORKSPACE", username, workspaceName })
  }, [username, workspaceName, scheduleSend])

  if (!username || !workspaceName) {
    return <Navigate to="/workspaces" />
  }

  if (
    scheduleState.matches("idle") ||
    scheduleState.matches("gettingWorkspace") ||
    scheduleState.matches("gettingPermissions") ||
    !workspace
  ) {
    return <FullScreenLoader />
  }

  if (scheduleState.matches("error")) {
    return (
      <AlertBanner
        severity="error"
        error={getWorkspaceError || checkPermissionsError}
        text={
          getWorkspaceError
            ? t("getWorkspaceError")
            : t("checkPermissionsError")
        }
        retry={() =>
          scheduleSend({ type: "GET_WORKSPACE", username, workspaceName })
        }
      />
    )
  }

  if (!permissions?.updateWorkspace) {
    return <AlertBanner severity="error" error={Error(t("forbiddenError"))} />
  }

  if (
    scheduleState.matches("presentForm") ||
    scheduleState.matches("submittingSchedule")
  ) {
    return (
      <WorkspaceScheduleForm
        submitScheduleError={submitScheduleError}
        initialValues={{
          ...getAutoStart(workspace),
          ...getAutoStop(workspace),
        }}
        isLoading={scheduleState.tags.has("loading")}
        onCancel={() => {
          navigate(`/@${username}/${workspaceName}`)
        }}
        onSubmit={(values) => {
          scheduleSend({
            type: "SUBMIT_SCHEDULE",
            autoStart: formValuesToAutoStartRequest(values),
            ttl: formValuesToTTLRequest(values),
            autoStartChanged: scheduleChanged(getAutoStart(workspace), values),
            autoStopChanged: scheduleChanged(getAutoStop(workspace), values),
          })
        }}
      />
    )
  }

  if (scheduleState.matches("showingRestartDialog")) {
    return (
      <ConfirmDialog
        open
        title={t("dialogTitle")}
        description={t("dialogDescription")}
        confirmText={t("restart")}
        cancelText={t("applyLater")}
        hideCancel={false}
        onConfirm={() => {
          scheduleSend("RESTART_WORKSPACE")
        }}
        onClose={() => {
          scheduleSend("APPLY_LATER")
        }}
      />
    )
  }

  if (scheduleState.matches("done")) {
    return (
      <Navigate
        to={`/@${username}/${workspaceName}`}
        state={{ shouldRestartWorkspace }}
      />
    )
  }

  // Theoretically impossible - log and bail
  console.error("WorkspaceSchedulePage: unknown state :: ", scheduleState)
  return <Navigate to="/" />
}

export default WorkspaceSchedulePage
