import Button from "@material-ui/core/Button"
import IconButton from "@material-ui/core/IconButton"
import Popover from "@material-ui/core/Popover"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import AddIcon from "@material-ui/icons/Add"
import RemoveIcon from "@material-ui/icons/Remove"
import ScheduleIcon from "@material-ui/icons/Schedule"
import { Maybe } from "components/Conditionals/Maybe"
import { Stack } from "components/Stack/Stack"
import dayjs from "dayjs"
import advancedFormat from "dayjs/plugin/advancedFormat"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import timezone from "dayjs/plugin/timezone"
import utc from "dayjs/plugin/utc"
import { useRef, useState } from "react"
import { useTranslation } from "react-i18next"
import { colors } from "theme/colors"
import { Workspace } from "../../api/typesGenerated"
import { isWorkspaceOn } from "../../util/workspace"
import { WorkspaceSchedule } from "../WorkspaceSchedule/WorkspaceSchedule"
import { EditHours } from "./EditHours"
import { WorkspaceScheduleLabel } from "./WorkspaceScheduleLabel"

// REMARK: some plugins depend on utc, so it's listed first. Otherwise they're
//         sorted alphabetically.
dayjs.extend(utc)
dayjs.extend(advancedFormat)
dayjs.extend(duration)
dayjs.extend(relativeTime)
dayjs.extend(timezone)

export const canEditDeadline = (workspace: Workspace): boolean => {
  return isWorkspaceOn(workspace) && Boolean(workspace.latest_build.deadline)
}

export const shouldDisplayScheduleLabel = (workspace: Workspace): boolean => {
  if (canEditDeadline(workspace)) {
    return true
  }
  if (isWorkspaceOn(workspace)) {
    return false
  }
  return Boolean(workspace.autostart_schedule)
}

export interface WorkspaceScheduleButtonProps {
  workspace: Workspace
  onDeadlinePlus: (hours: number) => void
  onDeadlineMinus: (hours: number) => void
  maxDeadlineIncrease: number
  maxDeadlineDecrease: number
  canUpdateWorkspace: boolean
}

export type EditMode = "add" | "subtract" | "off"

export const WorkspaceScheduleButton: React.FC<
  WorkspaceScheduleButtonProps
> = ({
  workspace,
  onDeadlinePlus,
  onDeadlineMinus,
  maxDeadlineDecrease,
  maxDeadlineIncrease,
  canUpdateWorkspace,
}) => {
  const { t } = useTranslation("workspacePage")
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const [editMode, setEditMode] = useState<EditMode>("off")
  const id = isOpen ? "schedule-popover" : undefined
  const styles = useStyles({ editMode })
  const deadlinePlusEnabled = maxDeadlineIncrease >= 1
  const deadlineMinusEnabled = maxDeadlineDecrease >= 1

  const onClose = () => {
    setIsOpen(false)
  }

  const handleSubmitHours = (hours: number) => {
    if (hours !== 0) {
      if (editMode === "add") {
        onDeadlinePlus(hours)
      }
      if (editMode === "subtract") {
        onDeadlineMinus(hours)
      }
    }
    setEditMode("off")
  }

  return (
    <span className={styles.wrapper}>
      <Maybe condition={shouldDisplayScheduleLabel(workspace)}>
        <Stack
          className={styles.label}
          spacing={1}
          direction="row"
          alignItems="center"
        >
          <Stack spacing={1} direction="row" alignItems="center">
            <WorkspaceScheduleLabel workspace={workspace} />
            <Maybe condition={canUpdateWorkspace && canEditDeadline(workspace)}>
              <span className={styles.actions}>
                <IconButton
                  className={styles.subtractButton}
                  aria-label="Subtract hours from deadline"
                  size="small"
                  disabled={!deadlineMinusEnabled}
                  onClick={() => {
                    setEditMode("subtract")
                  }}
                >
                  <Tooltip
                    title={t("workspaceScheduleButton.editDeadlineMinus")}
                  >
                    <RemoveIcon />
                  </Tooltip>
                </IconButton>
                <IconButton
                  className={styles.addButton}
                  aria-label="Add hours to deadline"
                  size="small"
                  disabled={!deadlinePlusEnabled}
                  onClick={() => {
                    setEditMode("add")
                  }}
                >
                  <Tooltip
                    title={t("workspaceScheduleButton.editDeadlinePlus")}
                  >
                    <AddIcon />
                  </Tooltip>
                </IconButton>
              </span>
            </Maybe>
          </Stack>
          <Maybe
            condition={
              canUpdateWorkspace &&
              canEditDeadline(workspace) &&
              editMode !== "off"
            }
          >
            <EditHours
              handleSubmit={handleSubmitHours}
              max={
                editMode === "add" ? maxDeadlineIncrease : maxDeadlineDecrease
              }
            />
          </Maybe>
        </Stack>
      </Maybe>
      <>
        <Button
          variant="outlined"
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

interface StyleProps {
  editMode: EditMode
}

const useStyles = makeStyles<Theme, StyleProps>((theme) => ({
  wrapper: {
    display: "inline-flex",
    alignItems: "center",

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
    },
  },
  label: {
    padding: theme.spacing(0, 2),
    color: theme.palette.text.secondary,
    borderRadius: theme.shape.borderRadius,
    border: `1px solid ${colors.gray[11]}`, // Same as outlined button
    display: "flex",
    height: theme.spacing(5), // Same as button
    alignItems: "center",
    borderTopRightRadius: 0,
    borderBottomRightRadius: 0,
    borderRight: 0,

    [theme.breakpoints.down("sm")]: {
      width: "100%",
      padding: theme.spacing(1.5, 2),
      flexDirection: "column",
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
    flexShrink: 0,

    "&.label": {
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
  addButton: {
    borderRadius: theme.shape.borderRadius,
    border: ({ editMode }) =>
      editMode === "add"
        ? `2px solid ${theme.palette.primary.main}`
        : "2px solid transparent",
  },
  subtractButton: {
    borderRadius: theme.shape.borderRadius,
    border: ({ editMode }) =>
      editMode === "subtract"
        ? `2px solid ${theme.palette.primary.main}`
        : "2px solid transparent",
  },
  popoverPaper: {
    padding: `${theme.spacing(2)}px ${theme.spacing(3)}px ${theme.spacing(
      3,
    )}px`,
  },
}))
