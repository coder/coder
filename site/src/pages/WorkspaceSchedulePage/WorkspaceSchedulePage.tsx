import { useMachine, useSelector } from "@xstate/react"
import { defaultSchedule, emptySchedule, scheduleToAutoStart } from "pages/WorkspacesPage/schedule"
import { defaultTTL, emptyTTL, ttlMsToAutoStop } from "pages/WorkspacesPage/ttl"
import React, { useContext, useEffect, useState } from "react"
import { Navigate, useNavigate, useParams } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import {
  WorkspaceScheduleForm,
  WorkspaceScheduleFormValues,
} from "../../components/WorkspaceScheduleForm/WorkspaceScheduleForm"
import { firstOrItem } from "../../util/array"
import { selectUser } from "../../xServices/auth/authSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { workspaceSchedule } from "../../xServices/workspaceSchedule/workspaceScheduleXService"

const Language = {
  forbiddenError: "You don't have permissions to update the schedule for this workspace.",
  getWorkspaceError: "Failed to fetch workspace.",
  checkPermissionsError: "Failed to fetch permissions.",
}

export const formValuesToAutoStartRequest = (
  values: WorkspaceScheduleFormValues,
): TypesGen.UpdateWorkspaceAutostartRequest => {
  if (!values.startTime) {
    return {
      schedule: "",
    }
  }

  const [HH, mm] = values.startTime.split(":")

  // Note: Space after CRON_TZ if timezone is defined
  const preparedTZ = values.timezone ? `CRON_TZ=${values.timezone} ` : ""

  const makeCronString = (dow: string) => `${preparedTZ}${mm} ${HH} * * ${dow}`

  const days = [
    values.sunday,
    values.monday,
    values.tuesday,
    values.wednesday,
    values.thursday,
    values.friday,
    values.saturday,
  ]

  const isEveryDay = days.every((day) => day)

  const isMonThroughFri =
    !values.sunday &&
    values.monday &&
    values.tuesday &&
    values.wednesday &&
    values.thursday &&
    values.friday &&
    !values.saturday &&
    !values.sunday

  // Handle special cases, falling through to comma-separation
  if (isEveryDay) {
    return {
      schedule: makeCronString("*"),
    }
  } else if (isMonThroughFri) {
    return {
      schedule: makeCronString("1-5"),
    }
  } else {
    const dow = days.reduce((previous, current, idx) => {
      if (!current) {
        return previous
      } else {
        const prefix = previous ? "," : ""
        return previous + prefix + idx
      }
    }, "")

    return {
      schedule: makeCronString(dow),
    }
  }
}

export const formValuesToTTLRequest = (
  values: WorkspaceScheduleFormValues,
): TypesGen.UpdateWorkspaceTTLRequest => {
  return {
    // minutes to nanoseconds
    ttl_ms: values.ttl ? values.ttl * 60 * 60 * 1000 : undefined,
  }
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

  const onToggleAutoStart = () => {
    if (autoStart.enabled) {
      setAutoStart({
        enabled: false,
        schedule: emptySchedule,
      })
    } else {
      if (workspace?.autostart_schedule) {
        // repopulate saved schedule
        setAutoStart({
          enabled: true,
          schedule: getAutoStart(workspace).schedule,
        })
      } else {
        // populate with defaults
        setAutoStart({
          enabled: true,
          schedule: defaultSchedule(),
        })
      }
    }
  }

  const onToggleAutoStop = () => {
    if (autoStop.enabled) {
      setAutoStop({
        enabled: false,
        ttl: emptyTTL,
      })
    } else {
      if (workspace?.ttl_ms) {
        // repopulate saved ttl
        setAutoStop({
          enabled: true,
          ttl: getAutoStop(workspace).ttl,
        })
      } else {
        // set default
        setAutoStop({
          enabled: true,
          ttl: defaultTTL,
        })
      }
    }
  }

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
        autoStart={autoStart}
        toggleAutoStart={onToggleAutoStart}
        autoStop={autoStop}
        toggleAutoStop={onToggleAutoStop}
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
