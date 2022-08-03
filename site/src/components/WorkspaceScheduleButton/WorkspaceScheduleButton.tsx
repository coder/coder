import Button from "@material-ui/core/Button"
import IconButton from "@material-ui/core/IconButton"
import Popover from "@material-ui/core/Popover"
import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import AddIcon from "@material-ui/icons/Add"
import RemoveIcon from "@material-ui/icons/Remove"
import ScheduleIcon from "@material-ui/icons/Schedule"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { useRef, useState } from "react"
import { Workspace } from "../../api/typesGenerated"
import { isWorkspaceOn } from "../../util/workspace"
import { WorkspaceSchedule } from "../WorkspaceSchedule/WorkspaceSchedule"
import { WorkspaceScheduleLabel } from "./WorkspaceScheduleLabel"

// REMARK: some plugins depend on utc, so it's listed first. Otherwise they're
//         sorted alphabetically.
dayjs.extend(utc)
dayjs.extend(advancedFormat)
dayjs.extend(duration)
dayjs.extend(relativeTime)
dayjs.extend(timezone)

export const Language = {
  schedule: "Schedule",
  editDeadlineMinus: "Subtract one hour",
  editDeadlinePlus: "Add one hour",
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

export interface WorkspaceScheduleButtonProps {
  workspace: Workspace
  onDeadlinePlus: () => void
  onDeadlineMinus: () => void
  canUpdateWorkspace: boolean
}

export const WorkspaceScheduleButton: React.FC<WorkspaceScheduleButtonProps> = ({
  workspace,
  onDeadlinePlus,
  onDeadlineMinus,
  canUpdateWorkspace,
}) => {
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "schedule-popover" : undefined
  const styles = useStyles()

  const onClose = () => {
    setIsOpen(false)
  }

  return (
    <span className={styles.wrapper}>
      <span className={styles.label}>
        <WorkspaceScheduleLabel workspace={workspace} />
        {canUpdateWorkspace && shouldDisplayPlusMinus(workspace) && (
          <span className={styles.actions}>
            <IconButton
              className={styles.iconButton}
              size="small"
              disabled={deadlineMinusDisabled(workspace, dayjs())}
              onClick={onDeadlineMinus}
            >
              <Tooltip title={Language.editDeadlineMinus}>
                <RemoveIcon />
              </Tooltip>
            </IconButton>
            <IconButton
              className={styles.iconButton}
              size="small"
              disabled={deadlinePlusDisabled(workspace, dayjs())}
              onClick={onDeadlinePlus}
            >
              <Tooltip title={Language.editDeadlinePlus}>
                <AddIcon />
              </Tooltip>
            </IconButton>
          </span>
        )}
      </span>
      <>
        <Button
          ref={anchorRef}
          startIcon={<ScheduleIcon />}
          onClick={() => {
            setIsOpen(true)
          }}
          className={styles.scheduleButton}
        >
          {Language.schedule}
        </Button>
        <Popover
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
          <WorkspaceSchedule workspace={workspace} canUpdateWorkspace={canUpdateWorkspace} />
        </Popover>
      </>
    </span>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    display: "inline-flex",
    alignItems: "center",
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: `${theme.shape.borderRadius}px`,

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
    },
  },
  label: {
    borderRight: 0,
    padding: "0 8px 0 16px",
    color: theme.palette.text.secondary,

    [theme.breakpoints.down("sm")]: {
      width: "100%",
      display: "flex",
      alignItems: "center",
      padding: theme.spacing(1.5, 2),
    },
  },
  actions: {
    [theme.breakpoints.down("sm")]: {
      marginLeft: "auto",
      display: "flex",
      paddingLeft: theme.spacing(1),
      marginRight: -theme.spacing(1),
    },
  },
  scheduleButton: {
    border: "none",
    borderLeft: `1px solid ${theme.palette.divider}`,
    borderRadius: `0px ${theme.shape.borderRadius}px ${theme.shape.borderRadius}px 0px`,
    flexShrink: 0,

    [theme.breakpoints.down("sm")]: {
      width: "100%",
      borderLeft: 0,
      borderTop: `1px solid ${theme.palette.divider}`,
      borderRadius: `0 0 ${theme.shape.borderRadius}px ${theme.shape.borderRadius}px`,
    },
  },
  iconButton: {
    borderRadius: 2,
  },
  popoverPaper: {
    padding: `${theme.spacing(2)}px ${theme.spacing(3)}px ${theme.spacing(3)}px`,
  },
}))
