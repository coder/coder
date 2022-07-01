import cronstrue from "cronstrue"
import dayjs from "dayjs"
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
export const extractTimezone = (raw: string, defaultTZ = DEFAULT_TIMEZONE): string => {
  const matches = raw.match(/CRON_TZ=\S*\s/g)

  if (matches && matches.length) {
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
  autoStartLabel: "Starts at",
  autoStopLabel: "Stops at",
}

export const autoStartDisplay = (schedule: string | undefined): string => {
  if (schedule) {
    return (
      cronstrue
        .toString(stripTimezone(schedule), { throwExceptionOnParseError: false })
        // We don't want to keep the At because it is on the label
        .replace("At", "")
    )
  } else {
    return Language.manual
  }
}

export const autoStopDisplay = (workspace: Workspace): string => {
  const deadline = dayjs(workspace.latest_build.deadline).utc()
  // a manual shutdown has a deadline of '"0001-01-01T00:00:00Z"'
  // SEE: #1834
  const hasDeadline = deadline.year() > 1
  const ttl = workspace.ttl_ms

  if (isWorkspaceOn(workspace) && hasDeadline) {
    // Workspace is on --> derive from latest_build.deadline. Note that the
    // user may modify their workspace object (ttl) while the workspace is
    // running and depending on system semantics, the deadline may still
    // represent the previously defined ttl. Thus, we always derive from the
    // deadline as the source of truth.
    const now = dayjs().utc()
    if (now.isAfter(deadline)) {
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
