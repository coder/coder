import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import ScheduleIcon from "@material-ui/icons/Schedule"
import cronstrue from "cronstrue"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { Workspace } from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { stripTimezone } from "../../util/schedule"
import { isWorkspaceOn } from "../../util/workspace"
import { Stack } from "../Stack/Stack"

// REMARK: some plugins depend on utc, so it's listed first. Otherwise they're
//         sorted alphabetically.
dayjs.extend(utc)
dayjs.extend(advancedFormat)
dayjs.extend(duration)
dayjs.extend(relativeTime)
dayjs.extend(timezone)

export const Language = {
  autoStartDisplay: (schedule: string | undefined): string => {
    if (schedule) {
      return cronstrue.toString(stripTimezone(schedule), { throwExceptionOnParseError: false })
    } else {
      return "Manual"
    }
  },
  autoStartLabel: "START",
  autoStopLabel: "SHUTDOWN",
  autoStopDisplay: (workspace: Workspace): string => {
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
        return "Workspace is shutting down"
      } else {
        return deadline.tz(dayjs.tz.guess()).format("MMM D, YYYY h:mm A")
      }
    } else if (!ttl || ttl < 1) {
      // If the workspace is not on, and the ttl is 0 or undefined, then the
      // workspace is set to manually shutdown.
      return "Manual"
    } else {
      // The workspace has a ttl set, but is either in an unknown state or is
      // not running. Therefore, we derive from workspace.ttl.
      const duration = dayjs.duration(ttl, "milliseconds")
      return `${duration.humanize()} after start`
    }
  },
  editScheduleLink: "Edit schedule",
  schedule: `Schedule (${dayjs.tz.guess()})`,
}

export interface WorkspaceScheduleProps {
  workspace: Workspace
}

export const WorkspaceSchedule: FC<WorkspaceScheduleProps> = ({ workspace }) => {
  const styles = useStyles()

  return (
    <div className={styles.schedule}>
      <Stack spacing={2}>
        <Typography variant="body1" className={styles.title}>
          <ScheduleIcon className={styles.scheduleIcon} />
          {Language.schedule}
        </Typography>
        <div>
          <span className={styles.scheduleLabel}>{Language.autoStartLabel}</span>
          <span className={styles.scheduleValue} data-chromatic="ignore">
            {Language.autoStartDisplay(workspace.autostart_schedule)}
          </span>
        </div>
        <div>
          <span className={styles.scheduleLabel}>{Language.autoStopLabel}</span>
          <span className={styles.scheduleValue} data-chromatic="ignore">
            {Language.autoStopDisplay(workspace)}
          </span>
        </div>
        <div>
          <Link
            className={styles.scheduleAction}
            component={RouterLink}
            to={`/@${workspace.owner_name}/${workspace.name}/schedule`}
          >
            {Language.editScheduleLink}
          </Link>
        </div>
      </Stack>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  schedule: {
    fontFamily: MONOSPACE_FONT_FAMILY,
  },
  title: {
    fontWeight: 600,

    fontFamily: "inherit",
    display: "flex",
    alignItems: "center",
  },
  scheduleIcon: {
    width: 16,
    height: 16,
    marginRight: theme.spacing(1),
  },
  scheduleLabel: {
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
    color: theme.palette.text.secondary,
  },
  scheduleValue: {
    fontSize: 16,
    marginTop: theme.spacing(0.25),
    display: "inline-block",
    color: theme.palette.text.secondary,
  },
  scheduleAction: {
    cursor: "pointer",
  },
}))
