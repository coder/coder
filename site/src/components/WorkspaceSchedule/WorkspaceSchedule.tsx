import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import ScheduleIcon from "@material-ui/icons/Schedule"
import cronstrue from "cronstrue"
import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import { Link as RouterLink } from "react-router-dom"
import { Workspace } from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { extractTimezone, stripTimezone } from "../../util/schedule"
import { Stack } from "../Stack/Stack"

dayjs.extend(duration)
dayjs.extend(relativeTime)

const Language = {
  autoStartDisplay: (schedule: string): string => {
    if (schedule) {
      return cronstrue.toString(stripTimezone(schedule), { throwExceptionOnParseError: false })
    }
    return "Manual"
  },
  autoStartLabel: (schedule: string): string => {
    const prefix = "Start"

    if (schedule) {
      return `${prefix} (${extractTimezone(schedule)})`
    } else {
      return prefix
    }
  },
  autoStopDisplay: (workspace: Workspace): string => {
    const latest = workspace.latest_build

    if (!workspace.ttl || workspace.ttl < 1) {
      return "Manual"
    }

    if (latest.transition === "start") {
      const now = dayjs()
      const updatedAt = dayjs(latest.updated_at)
      const deadline = updatedAt.add(workspace.ttl / 1_000_000, "ms")
      if (now.isAfter(deadline)) {
        return "Workspace is shutting down now"
      }
      return now.to(deadline)
    }

    const duration = dayjs.duration(workspace.ttl / 1_000_000, "milliseconds")
    return `${duration.humanize()} after start`
  },
  editScheduleLink: "Edit schedule",
  schedule: "Schedule",
}

export interface WorkspaceScheduleProps {
  workspace: Workspace
}

export const WorkspaceSchedule: React.FC<WorkspaceScheduleProps> = ({ workspace }) => {
  const styles = useStyles()

  return (
    <div className={styles.schedule}>
      <Stack spacing={2}>
        <Typography variant="body1" className={styles.title}>
          <ScheduleIcon className={styles.scheduleIcon} />
          {Language.schedule}
        </Typography>
        <div>
          <span className={styles.scheduleLabel}>{Language.autoStartLabel(workspace.autostart_schedule)}</span>
          <span className={styles.scheduleValue}>{Language.autoStartDisplay(workspace.autostart_schedule)}</span>
        </div>
        <div>
          <span className={styles.scheduleLabel}>Shutdown</span>
          <span className={styles.scheduleValue}>{Language.autoStopDisplay(workspace)}</span>
        </div>
        <div>
          <Link className={styles.scheduleAction} component={RouterLink} to={`/workspaces/${workspace.id}/schedule`}>
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
