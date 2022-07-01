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
import { Stack } from "../Stack/Stack"
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
}

export const WorkspaceScheduleButton: React.FC<WorkspaceScheduleButtonProps> = ({
  workspace,
  onDeadlinePlus,
  onDeadlineMinus,
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
    // It is from the button props
    minHeight: 42,
  },

  iconButton: {
    borderRadius: 2,
  },

  popoverPaper: {
    padding: `${theme.spacing(2)}px ${theme.spacing(3)}px ${theme.spacing(3)}px`,
  },
}))
