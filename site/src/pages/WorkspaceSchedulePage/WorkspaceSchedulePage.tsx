import { useMachine, useSelector } from "@xstate/react"
import * as cronParser from "cron-parser"
import dayjs from "dayjs"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import React, { useContext, useEffect } from "react"
import { useNavigate, useParams } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import {
  defaultWorkspaceSchedule,
  defaultWorkspaceScheduleTTL,
  WorkspaceScheduleForm,
  WorkspaceScheduleFormValues,
} from "../../components/WorkspaceScheduleForm/WorkspaceScheduleForm"
import { firstOrItem } from "../../util/array"
import { extractTimezone, stripTimezone } from "../../util/schedule"
import { selectUser } from "../../xServices/auth/authSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { workspaceSchedule } from "../../xServices/workspaceSchedule/workspaceScheduleXService"

// REMARK: timezone plugin depends on UTC
//
// SEE: https://day.js.org/docs/en/timezone/timezone
dayjs.extend(utc)
dayjs.extend(timezone)

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

export const workspaceToInitialValues = (
  workspace: TypesGen.Workspace,
  defaultTimeZone = "",
): WorkspaceScheduleFormValues => {
  const schedule = workspace.autostart_schedule
  const ttlHours = workspace.ttl_ms
    ? Math.round(workspace.ttl_ms / (1000 * 60 * 60))
    : defaultWorkspaceScheduleTTL

  if (!schedule) {
    return defaultWorkspaceSchedule(ttlHours, defaultTimeZone)
  }

  const timezone = extractTimezone(schedule, defaultTimeZone)

  const expression = cronParser.parseExpression(stripTimezone(schedule))

  const HH = expression.fields.hour.join("").padStart(2, "0")
  const mm = expression.fields.minute.join("").padStart(2, "0")

  const weeklyFlags = [false, false, false, false, false, false, false]

  for (const day of expression.fields.dayOfWeek) {
    weeklyFlags[day % 7] = true
  }

  return {
    sunday: weeklyFlags[0],
    monday: weeklyFlags[1],
    tuesday: weeklyFlags[2],
    wednesday: weeklyFlags[3],
    thursday: weeklyFlags[4],
    friday: weeklyFlags[5],
    saturday: weeklyFlags[6],
    startTime: `${HH}:${mm}`,
    timezone,
    ttl: ttlHours,
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

  if (!username || !workspaceName) {
    navigate("/workspaces")
    return null
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
        initialValues={workspaceToInitialValues(workspace, dayjs.tz.guess())}
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
    navigate(`/@${username}/${workspaceName}`)
    return <FullScreenLoader />
  }

  // Theoretically impossible - log and bail
  console.error("WorkspaceSchedulePage: unknown state :: ", scheduleState)
  navigate("/")
  return null
}
