import { useMachine, useSelector } from "@xstate/react"
import { scheduleToAutoStart } from "pages/WorkspaceSchedulePage/schedule"
import { ttlMsToAutoStop } from "pages/WorkspaceSchedulePage/ttl"
import React, { useContext, useEffect, useState } from "react"
import { Navigate, useNavigate, useParams } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { WorkspaceScheduleForm } from "../../components/WorkspaceScheduleForm/WorkspaceScheduleForm"
import { firstOrItem } from "../../util/array"
import { selectUser } from "../../xServices/auth/authSelectors"
import { XServiceContext } from "../../xServices/StateContext"
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

  const xServices = useContext(XServiceContext)
  const me = useSelector(xServices.authXService, selectUser)

  const [scheduleState, scheduleSend] = useMachine(workspaceSchedule, {
    context: {
      userId: me?.id,
    },
  })
  const {
    checkPermissionsError,
    submitScheduleError,
    getWorkspaceError,
    permissions,
    workspace,
    template,
  } = scheduleState.context

  // Get workspace on mount and whenever the args for getting a workspace change.
  // scheduleSend should not change.
  useEffect(() => {
    username && workspaceName && scheduleSend({ type: "GET_WORKSPACE", username, workspaceName })
  }, [username, workspaceName, scheduleSend])

  const getAutoStart = (workspace?: TypesGen.Workspace) =>
    scheduleToAutoStart(workspace?.autostart_schedule)
  const getAutoStop = (workspace?: TypesGen.Workspace) => ttlMsToAutoStop(workspace?.ttl_ms)

  const getMaxTTLms = (template?: TypesGen.Template) => template?.max_ttl_ms

  const [autoStart, setAutoStart] = useState(getAutoStart(workspace))
  const [autoStop, setAutoStop] = useState(getAutoStop(workspace))
  const [maxTTLms, setMaxTTL] = useState(getMaxTTLms(template))

  useEffect(() => {
    setAutoStart(getAutoStart(workspace))
    setAutoStop(getAutoStop(workspace))
  }, [workspace])

  useEffect(() => {
    setMaxTTL(getMaxTTLms(template))
  }, [template])

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
      <ErrorSummary
        error={getWorkspaceError || checkPermissionsError}
        defaultMessage={
          getWorkspaceError ? Language.getWorkspaceError : Language.checkPermissionsError
        }
        retry={() => scheduleSend({ type: "GET_WORKSPACE", username, workspaceName })}
      />
    )
  }

  if (!permissions?.updateWorkspace) {
    return <ErrorSummary error={Error(Language.forbiddenError)} />
  }

  if (scheduleState.matches("presentForm") || scheduleState.matches("submittingSchedule")) {
    return (
      <WorkspaceScheduleForm
        submitScheduleError={submitScheduleError}
        initialValues={{ ...autoStart, ...autoStop }}
        maxTTLms={maxTTLms}
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
