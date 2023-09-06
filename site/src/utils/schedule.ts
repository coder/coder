import cronstrue from "cronstrue"
import dayjs, { Dayjs } from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { Workspace } from "../api/typesGenerated"
import { isWorkspaceOn } from "./workspace"

// REMARK: some plugins depend on utc, so it's listed first. Otherwise they're
//         sorted alphabetically.
dayjs.extend(utc)
dayjs.extend(advancedFormat)
dayjs.extend(duration)
dayjs.extend(relativeTime)
dayjs.extend(timezone)

/**
 * @fileoverview Client-side counterpart of the coderd/autostart/schedule Go
 * package. This package is a variation on crontab that uses minute, hour and
 * day of week.
 */

/**
 * DEFAULT_TIMEZONE is the default timezone that crontab assumes unless one is
 * specified.
 */
const DEFAULT_TIMEZONE = "UTC"

/**
 * stripTimezone strips a leading timezone from a schedule string
 */
export const stripTimezone = (raw: string): string => {
  return raw.replace(/CRON_TZ=\S*\s/, "")
}

/**
 * extractTimezone returns a leading timezone from a schedule string if one is
 * specified; otherwise the specified defaultTZ
 */
export const extractTimezone = (
  raw: string,
  defaultTZ = DEFAULT_TIMEZONE,
): string => {
  const matches = raw.match(/CRON_TZ=\S*\s/g)

  if (matches && matches.length > 0) {
    return matches[0].replace(/CRON_TZ=/, "").trim()
  } else {
    return defaultTZ
  }
}

/** Language used in the schedule components */
export const Language = {
  manual: "Manual",
  workspaceShuttingDownLabel: "Workspace is shutting down",
  afterStart: "after start",
  autostartLabel: "Starts at",
  autostopLabel: "Stops at",
}

export const autostartDisplay = (schedule: string | undefined): string => {
  if (schedule) {
    return (
      cronstrue
        .toString(stripTimezone(schedule), {
          throwExceptionOnParseError: false,
        })
        // We don't want to keep the At because it is on the label
        .replace("At", "")
    )
  } else {
    return Language.manual
  }
}

export const isShuttingDown = (
  workspace: Workspace,
  deadline?: Dayjs,
): boolean => {
  if (!deadline) {
    if (!workspace.latest_build.deadline) {
      return false
    }
    deadline = dayjs(workspace.latest_build.deadline).utc()
  }
  const now = dayjs().utc()
  return isWorkspaceOn(workspace) && now.isAfter(deadline)
}

export const autostopDisplay = (workspace: Workspace): string => {
  const ttl = workspace.ttl_ms

  if (isWorkspaceOn(workspace) && workspace.latest_build.deadline) {
    // Workspace is on --> derive from latest_build.deadline. Note that the
    // user may modify their workspace object (ttl) while the workspace is
    // running and depending on system semantics, the deadline may still
    // represent the previously defined ttl. Thus, we always derive from the
    // deadline as the source of truth.

    const deadline = dayjs(workspace.latest_build.deadline).utc()
    if (isShuttingDown(workspace, deadline)) {
      return Language.workspaceShuttingDownLabel
    } else {
      return deadline.tz(dayjs.tz.guess()).format("MMM D, YYYY h:mm A")
    }
  } else if (!ttl || ttl < 1) {
    // If the workspace is not on, and the ttl is 0 or undefined, then the
    // workspace is set to manually shutdown.
    return Language.manual
  } else {
    // The workspace has a ttl set, but is either in an unknown state or is
    // not running. Therefore, we derive from workspace.ttl.
    const duration = dayjs.duration(ttl, "milliseconds")
    return `${duration.humanize()} ${Language.afterStart}`
  }
}

export const deadlineExtensionMin = dayjs.duration(30, "minutes")
export const deadlineExtensionMax = dayjs.duration(24, "hours")

/**
 * Depends on the time the workspace was last updated and a global constant.
 * @param ws workspace
 * @returns the latest datetime at which the workspace can be automatically shut down.
 */
export function getMaxDeadline(ws: Workspace | undefined): dayjs.Dayjs {
  // note: we count runtime from updated_at as started_at counts from the start of
  // the workspace build process, which can take a while.
  if (ws === undefined) {
    throw Error("Cannot calculate max deadline because workspace is undefined")
  }
  const startedAt = dayjs(ws.latest_build.updated_at)
  return startedAt.add(deadlineExtensionMax)
}

/**
 * Depends on the current time and a global constant.
 * @returns the earliest datetime at which the workspace can be automatically shut down.
 */
export function getMinDeadline(): dayjs.Dayjs {
  return dayjs().add(deadlineExtensionMin)
}

export const getDeadline = (workspace: Workspace): dayjs.Dayjs =>
  dayjs(workspace.latest_build.deadline).utc()

/**
 * Get number of hours you can add or subtract to the current deadline before hitting the max or min deadline.
 * @param deadline
 * @param workspace
 * @returns number, in hours
 */
export const getMaxDeadlineChange = (
  deadline: dayjs.Dayjs,
  extremeDeadline: dayjs.Dayjs,
): number => Math.abs(deadline.diff(extremeDeadline, "hours"))
