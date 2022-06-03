import { useMachine } from "@xstate/react"
import * as cronParser from "cron-parser"
import dayjs from "dayjs"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import React, { useEffect } from "react"
import { useNavigate, useParams } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import {
  WorkspaceScheduleForm,
  WorkspaceScheduleFormValues,
} from "../../components/WorkspaceScheduleForm/WorkspaceScheduleForm"
import { firstOrItem } from "../../util/array"
import { extractTimezone, stripTimezone } from "../../util/schedule"
import { workspaceSchedule } from "../../xServices/workspaceSchedule/workspaceScheduleXService"

// REMARK: timezone plugin depends on UTC
//
// SEE: https://day.js.org/docs/en/timezone/timezone
dayjs.extend(utc)
dayjs.extend(timezone)

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

export const formValuesToTTLRequest = (values: WorkspaceScheduleFormValues): TypesGen.UpdateWorkspaceTTLRequest => {
  return {
    // minutes to nanoseconds
    ttl_ms: values.ttl ? values.ttl * 60 * 60 * 1000 : undefined,
  }
}

export const workspaceToInitialValues = (workspace: TypesGen.Workspace): WorkspaceScheduleFormValues => {
  const schedule = workspace.autostart_schedule
  const ttlHours = workspace.ttl_ms ? Math.round(workspace.ttl_ms / (1000 * 60 * 60)) : 0

  if (!schedule) {
    return {
      sunday: false,
      monday: false,
      tuesday: false,
      wednesday: false,
      thursday: false,
      friday: false,
      saturday: false,
      startTime: "",
      timezone: "",
      ttl: ttlHours,
    }
  }

  const timezone = extractTimezone(schedule, dayjs.tz.guess())

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
  const navigate = useNavigate()
  const { workspace: workspaceQueryParam } = useParams()
  const workspaceId = firstOrItem(workspaceQueryParam, null)
  const [scheduleState, scheduleSend] = useMachine(workspaceSchedule)
  const { formErrors, getWorkspaceError, workspace } = scheduleState.context

  // Get workspace on mount and whenever workspaceId changes.
  // scheduleSend should not change.
  useEffect(() => {
    workspaceId && scheduleSend({ type: "GET_WORKSPACE", workspaceId })
  }, [workspaceId, scheduleSend])

  if (!workspaceId) {
    navigate("/workspaces")
    return null
  } else if (scheduleState.matches("idle") || scheduleState.matches("gettingWorkspace") || !workspace) {
    return <FullScreenLoader />
  } else if (scheduleState.matches("error")) {
    return <ErrorSummary error={getWorkspaceError} retry={() => scheduleSend({ type: "GET_WORKSPACE", workspaceId })} />
  } else if (scheduleState.matches("presentForm") || scheduleState.matches("submittingSchedule")) {
    return (
      <WorkspaceScheduleForm
        fieldErrors={formErrors}
        initialValues={workspaceToInitialValues(workspace)}
        isLoading={scheduleState.tags.has("loading")}
        onCancel={() => {
          navigate(`/workspaces/${workspaceId}`)
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
  } else if (scheduleState.matches("submitSuccess")) {
    navigate(`/workspaces/${workspaceId}`)
    return <FullScreenLoader />
  } else {
    // Theoretically impossible - log and bail
    console.error("WorkspaceSchedulePage: unknown state :: ", scheduleState)
    navigate("/")
    return null
  }
}
