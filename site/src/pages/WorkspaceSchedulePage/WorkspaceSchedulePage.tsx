import { useMachine } from "@xstate/react"
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
import { dowToWeeklyFlag, extractTimezone, stripTimezone } from "../../util/schedule"
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
    ttl: values.ttl ? values.ttl * 60 * 60 * 1000 * 1_000_000 : undefined,
  }
}

export const workspaceToInitialValues = (workspace: TypesGen.Workspace): WorkspaceScheduleFormValues => {
  const schedule = workspace.autostart_schedule
  const ttl = workspace.ttl ? workspace.ttl / (1_000_000 * 1000 * 60 * 60) : 0

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
      ttl,
    }
  }

  const timezone = extractTimezone(schedule, dayjs.tz.guess())
  const cronString = stripTimezone(schedule)

  // parts has the following format: "mm HH * * dow"
  const parts = cronString.split(" ")

  // -> we skip month and day-of-month
  const mm = parts[0]
  const HH = parts[1]
  const dow = parts[4]

  const weeklyFlags = dowToWeeklyFlag(dow)

  return {
    sunday: weeklyFlags[0],
    monday: weeklyFlags[1],
    tuesday: weeklyFlags[2],
    wednesday: weeklyFlags[3],
    thursday: weeklyFlags[4],
    friday: weeklyFlags[5],
    saturday: weeklyFlags[6],
    startTime: `${HH.padStart(2, "0")}:${mm.padStart(2, "0")}`,
    timezone,
    ttl,
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
