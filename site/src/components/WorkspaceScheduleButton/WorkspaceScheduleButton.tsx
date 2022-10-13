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
import { useTranslation } from "react-i18next"
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

export const shouldDisplayPlusMinus = (workspace: Workspace): boolean => {
  return isWorkspaceOn(workspace) && Boolean(workspace.latest_build.deadline)
}

export const shouldDisplayScheduleLabel = (workspace: Workspace): boolean => {
  if (shouldDisplayPlusMinus(workspace)) {
    return true
  }
  if (isWorkspaceOn(workspace)) {
    return false
  }
  return Boolean(workspace.autostart_schedule)
}

export interface WorkspaceScheduleButtonProps {
  workspace: Workspace
  onDeadlinePlus: () => void
  onDeadlineMinus: () => void
  deadlineMinusEnabled: () => boolean
  deadlinePlusEnabled: () => boolean
  canUpdateWorkspace: boolean
}

export const WorkspaceScheduleButton: React.FC<
  WorkspaceScheduleButtonProps
> = ({
  workspace,
  onDeadlinePlus,
  onDeadlineMinus,
  deadlinePlusEnabled,
  deadlineMinusEnabled,
  canUpdateWorkspace,
}) => {
  const { t } = useTranslation("workspacePage")
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const id = isOpen ? "schedule-popover" : undefined
  const styles = useStyles()

  const onClose = () => {
    setIsOpen(false)
  }

  return (
    <span className={styles.wrapper}>
      {shouldDisplayScheduleLabel(workspace) && (
        <span className={styles.label}>
          <WorkspaceScheduleLabel workspace={workspace} />
          {canUpdateWorkspace && shouldDisplayPlusMinus(workspace) && (
            <span className={styles.actions}>
              <IconButton
                className={styles.iconButton}
                size="small"
                disabled={!deadlineMinusEnabled()}
                onClick={onDeadlineMinus}
              >
                <Tooltip title={t("workspaceScheduleButton.editDeadlineMinus")}>
                  <RemoveIcon />
                </Tooltip>
              </IconButton>
              <IconButton
                className={styles.iconButton}
                size="small"
                disabled={!deadlinePlusEnabled()}
                onClick={onDeadlinePlus}
              >
                <Tooltip title={t("workspaceScheduleButton.editDeadlinePlus")}>
                  <AddIcon />
                </Tooltip>
              </IconButton>
            </span>
          )}
        </span>
      )}
      <>
        <Button
          ref={anchorRef}
          startIcon={<ScheduleIcon />}
          onClick={() => {
            setIsOpen(true)
          }}
          className={`${styles.scheduleButton} ${
            shouldDisplayScheduleLabel(workspace) ? "label" : ""
          }`}
        >
          {t("workspaceScheduleButton.schedule")}
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
          <WorkspaceSchedule
            workspace={workspace}
            canUpdateWorkspace={canUpdateWorkspace}
          />
        </Popover>
      </>
    </span>
  )
}

const useStyles = makeStyles((theme) => ({
  wrapper: {
    display: "inline-flex",
    alignItems: "center",
    borderRadius: `${theme.shape.borderRadius}px`,
    border: `1px solid ${theme.palette.divider}`,

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
    borderRadius: `${theme.shape.borderRadius}px`,
    flexShrink: 0,

    "&.label": {
      borderLeft: `1px solid ${theme.palette.divider}`,
      borderRadius: `0px ${theme.shape.borderRadius}px ${theme.shape.borderRadius}px 0px`,
    },

    [theme.breakpoints.down("sm")]: {
      width: "100%",

      "&.label": {
        borderRadius: `0 0 ${theme.shape.borderRadius}px ${theme.shape.borderRadius}px`,
        borderLeft: 0,
        borderTop: `1px solid ${theme.palette.divider}`,
      },
    },
  },
  iconButton: {
    borderRadius: theme.shape.borderRadius,
  },
  popoverPaper: {
    padding: `${theme.spacing(2)}px ${theme.spacing(3)}px ${theme.spacing(
      3,
    )}px`,
  },
}))
