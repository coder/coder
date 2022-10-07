import { useMachine } from "@xstate/react"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { scheduleToAutoStart } from "pages/WorkspaceSchedulePage/schedule"
import { ttlMsToAutoStop } from "pages/WorkspaceSchedulePage/ttl"
import React, { useEffect, useState } from "react"
import { Navigate, useNavigate, useParams } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { WorkspaceScheduleForm } from "../../components/WorkspaceScheduleForm/WorkspaceScheduleForm"
import { firstOrItem } from "../../util/array"
import { workspaceSchedule } from "../../xServices/workspaceSchedule/workspaceScheduleXService"
import { formValuesToAutoStartRequest, formValuesToTTLRequest } from "./formToRequest"

const Language = {
  forbiddenError: "You don't have permissions to update the schedule for this workspace.",
  getWorkspaceError: "Failed to fetch workspace.",
  checkPermissionsError: "Failed to fetch permissions.",
}

export const WorkspaceSchedulePage: React.FC = () => {
  const { username: usernameQueryParam, workspace: workspaceQueryParam } = useParams()
  const navigate = useNavigate()
  const username = firstOrItem(usernameQueryParam, null)
  const workspaceName = firstOrItem(workspaceQueryParam, null)
  const [scheduleState, scheduleSend] = useMachine(workspaceSchedule)
  const { checkPermissionsError, submitScheduleError, getWorkspaceError, permissions, workspace } =
    scheduleState.context

  // Get workspace on mount and whenever the args for getting a workspace change.
  // scheduleSend should not change.
  useEffect(() => {
    username && workspaceName && scheduleSend({ type: "GET_WORKSPACE", username, workspaceName })
  }, [username, workspaceName, scheduleSend])

  const getAutoStart = (workspace?: TypesGen.Workspace) =>
    scheduleToAutoStart(workspace?.autostart_schedule)
  const getAutoStop = (workspace?: TypesGen.Workspace) => ttlMsToAutoStop(workspace?.ttl_ms)

  const [autoStart, setAutoStart] = useState(getAutoStart(workspace))
  const [autoStop, setAutoStop] = useState(getAutoStop(workspace))

  useEffect(() => {
    setAutoStart(getAutoStart(workspace))
    setAutoStop(getAutoStop(workspace))
  }, [workspace])

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
        text={getWorkspaceError ? Language.getWorkspaceError : Language.checkPermissionsError}
        retry={() => scheduleSend({ type: "GET_WORKSPACE", username, workspaceName })}
      />
    )
  }

  if (!permissions?.updateWorkspace) {
    return <AlertBanner severity="error" error={Error(Language.forbiddenError)} />
  }

  if (scheduleState.matches("presentForm") || scheduleState.matches("submittingSchedule")) {
    return (
      <WorkspaceScheduleForm
        submitScheduleError={submitScheduleError}
        initialValues={{ ...autoStart, ...autoStop }}
        isLoading={scheduleState.tags.has("loading")}
        onCancel={() => {
          navigate(`/@${username}/${workspaceName}`)
        }}
        onSubmit={(values) => {
          scheduleSend({
            type: "SUBMIT_SCHEDULE",
            autoStart: formValuesToAutoStartRequest(values),
            ttl: formValuesToTTLRequest(values),
          })
        }}
      />
    )
  }

  if (scheduleState.matches("submitSuccess")) {
    return <Navigate to={`/@${username}/${workspaceName}`} />
  }

  // Theoretically impossible - log and bail
  console.error("WorkspaceSchedulePage: unknown state :: ", scheduleState)
  return <Navigate to="/" />
}

export default WorkspaceSchedulePage
