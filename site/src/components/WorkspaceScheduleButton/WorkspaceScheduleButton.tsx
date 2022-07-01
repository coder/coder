import Button from "@material-ui/core/Button"
import IconButton from "@material-ui/core/IconButton"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import AddIcon from "@material-ui/icons/Add"
import RemoveIcon from "@material-ui/icons/Remove"
import ScheduleIcon from "@material-ui/icons/Schedule"
import cronstrue from "cronstrue"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { useRef, useState } from "react"
import { Workspace } from "../../api/typesGenerated"
import { extractTimezone, stripTimezone } from "../../util/schedule"
import { isWorkspaceOn } from "../../util/workspace"
import { Stack } from "../Stack/Stack"
import { WorkspaceSchedule } from "../WorkspaceSchedule/WorkspaceSchedule"

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
      return (
        cronstrue
          .toString(stripTimezone(schedule), { throwExceptionOnParseError: false })
          // We don't want to keep the At because it is on the label
          .replace("At", "")
      )
    } else {
      return "Manual"
    }
  },
  autoStartLabel: "Starts at",
  autoStopLabel: "Stops at",
  workspaceShuttingDownLabel: "Workspace is shutting down",
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
        return Language.workspaceShuttingDownLabel
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
  editDeadlineMinus: "Subtract one hour",
  editDeadlinePlus: "Add one hour",
  scheduleHeader: (workspace: Workspace): string => {
    const tz = workspace.autostart_schedule
      ? extractTimezone(workspace.autostart_schedule)
      : dayjs.tz.guess()
    return `Schedule (${tz})`
  },
}

export const shouldDisplayPlusMinus = (workspace: Workspace): boolean => {
  if (!isWorkspaceOn(workspace)) {
    return false
  }
  const deadline = dayjs(workspace.latest_build.deadline).utc()
  return deadline.year() > 1
}

export const deadlineMinusDisabled = (workspace: Workspace, now: dayjs.Dayjs): boolean => {
  const delta = dayjs(workspace.latest_build.deadline).diff(now)
  return delta <= 30 * 60 * 1000 // 30 minutes
}

export const deadlinePlusDisabled = (workspace: Workspace, now: dayjs.Dayjs): boolean => {
  const delta = dayjs(workspace.latest_build.deadline).diff(now)
  return delta >= 24 * 60 * 60 * 1000 // 24 hours
}

const WorkspaceScheduleLabel: React.FC<{ workspace: Workspace }> = ({ workspace }) => {
  const styles = useStyles()

  if (isWorkspaceOn(workspace)) {
    const stopLabel = Language.autoStopDisplay(workspace)
    const isShuttingDown = stopLabel === Language.workspaceShuttingDownLabel

    // If it is shutting down, we don't need to display the auto stop label
    return (
      <span className={styles.labelText}>
        {!isShuttingDown && (
          <strong className={styles.labelStrong}>{Language.autoStopLabel}</strong>
        )}
        {stopLabel}
      </span>
    )
  }

  return (
    <span className={styles.labelText}>
      <strong className={styles.labelStrong}>{Language.autoStartLabel}</strong>
      {Language.autoStartDisplay(workspace.autostart_schedule)}
    </span>
  )
}

interface WorkspaceScheduleProps {
  workspace: Workspace
  onDeadlinePlus: () => void
  onDeadlineMinus: () => void
  now?: dayjs.Dayjs
}

export const WorkspaceScheduleButton: React.FC<WorkspaceScheduleProps> = ({
  workspace,
  onDeadlinePlus,
  onDeadlineMinus,
  now,
}) => {
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "schedule-popover" : undefined
  const styles = useStyles()

  const onClose = () => {
    setIsOpen(false)
  }

  return (
    <div className={styles.wrapper}>
      <div className={styles.label}>
        <WorkspaceScheduleLabel workspace={workspace} />
        {shouldDisplayPlusMinus(workspace) && (
          <Stack direction="row" spacing={0}>
            <IconButton
              className={styles.iconButton}
              size="small"
              disabled={deadlineMinusDisabled(workspace, now ?? dayjs())}
              onClick={onDeadlineMinus}
            >
              <Tooltip title={Language.editDeadlineMinus}>
                <RemoveIcon />
              </Tooltip>
            </IconButton>
            <IconButton
              className={styles.iconButton}
              size="small"
              disabled={deadlinePlusDisabled(workspace, now ?? dayjs())}
              onClick={onDeadlinePlus}
            >
              <Tooltip title={Language.editDeadlinePlus}>
                <AddIcon />
              </Tooltip>
            </IconButton>
          </Stack>
        )}
      </div>
      <div>
        <Button
          ref={anchorRef}
          startIcon={<ScheduleIcon />}
          onClick={() => {
            setIsOpen(true)
          }}
        >
          Schedule
        </Button>
        <Popover
          //className={styles.popover}
          classes={{ paper: styles.popoverPaper }}
          id={id}
          open={isOpen}
          anchorEl={anchorRef.current}
          onClose={onClose}
          anchorOrigin={{
            vertical: "bottom",
            horizontal: "right",
          }}
          transformOrigin={{
            vertical: "top",
            horizontal: "right",
          }}
        >
          <WorkspaceSchedule workspace={workspace} />
        </Popover>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    display: "flex",
    alignItems: "center",
  },

  label: {
    border: `1px solid ${theme.palette.divider}`,
    borderRight: 0,
    height: "100%",
    display: "flex",
    alignItems: "center",
    padding: "0 8px 0 16px",
    color: theme.palette.text.secondary,
  },

  labelText: {
    marginRight: theme.spacing(2),
  },

  labelStrong: {
    marginRight: theme.spacing(0.25),
  },

  iconButton: {
    borderRadius: 2,
  },

  popoverPaper: {
    padding: `${theme.spacing(2)}px ${theme.spacing(3)}px ${theme.spacing(3)}px`,
  },
}))
