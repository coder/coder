import Link from "@material-ui/core/Link"
import { OutdatedHelpTooltip } from "components/Tooltips"
import { FC, useRef, useState } from "react"
import { Link as RouterLink } from "react-router-dom"
import { createDayString } from "utils/createDayString"
import {
  getDisplayWorkspaceBuildInitiatedBy,
  getDisplayWorkspaceTemplateName,
  isWorkspaceOn,
} from "utils/workspace"
import { Workspace } from "../../api/typesGenerated"
import { Stats, StatsItem } from "components/Stats/Stats"
import upperFirst from "lodash/upperFirst"
import { autostartDisplay, autostopDisplay } from "utils/schedule"
import IconButton from "@material-ui/core/IconButton"
import RemoveIcon from "@material-ui/icons/RemoveOutlined"
import { makeStyles } from "@material-ui/core/styles"
import AddIcon from "@material-ui/icons/AddOutlined"
import Popover from "@material-ui/core/Popover"
import TextField from "@material-ui/core/TextField"
import Button from "@material-ui/core/Button"

const Language = {
  workspaceDetails: "Workspace Details",
  templateLabel: "Template",
  statusLabel: "Workspace Status",
  versionLabel: "Version",
  lastBuiltLabel: "Last built",
  outdated: "Outdated",
  upToDate: "Up to date",
  byLabel: "Last built by",
  costLabel: "Daily cost",
}

export interface WorkspaceStatsProps {
  workspace: Workspace
  maxDeadlineIncrease: number
  maxDeadlineDecrease: number
  canUpdateWorkspace: boolean
  quota_budget?: number
  onDeadlinePlus: (hours: number) => void
  onDeadlineMinus: (hours: number) => void
  handleUpdate: () => void
}

export const WorkspaceStats: FC<WorkspaceStatsProps> = ({
  workspace,
  quota_budget,
  maxDeadlineDecrease,
  maxDeadlineIncrease,
  canUpdateWorkspace,
  handleUpdate,
  onDeadlineMinus,
  onDeadlinePlus,
}) => {
  const initiatedBy = getDisplayWorkspaceBuildInitiatedBy(
    workspace.latest_build,
  )
  const displayTemplateName = getDisplayWorkspaceTemplateName(workspace)
  const styles = useStyles()
  const deadlinePlusEnabled = maxDeadlineIncrease >= 1
  const deadlineMinusEnabled = maxDeadlineDecrease >= 1
  const addButtonRef = useRef<HTMLButtonElement>(null)
  const subButtonRef = useRef<HTMLButtonElement>(null)
  const [isAddingTime, setIsAddingTime] = useState(false)
  const [isSubTime, setIsSubTime] = useState(false)

  return (
    <>
      <Stats aria-label={Language.workspaceDetails}>
        <StatsItem
          label={Language.templateLabel}
          value={
            <Link
              component={RouterLink}
              to={`/templates/${workspace.template_name}`}
            >
              {displayTemplateName}
            </Link>
          }
        />
        <StatsItem
          label={Language.versionLabel}
          value={
            <>
              <Link
                component={RouterLink}
                to={`/templates/${workspace.template_name}/versions/${workspace.latest_build.template_version_name}`}
              >
                {workspace.latest_build.template_version_name}
              </Link>

              {workspace.outdated && (
                <OutdatedHelpTooltip
                  onUpdateVersion={handleUpdate}
                  ariaLabel="update version"
                />
              )}
            </>
          }
        />
        <StatsItem
          label={Language.lastBuiltLabel}
          value={
            <>
              {upperFirst(createDayString(workspace.latest_build.created_at))}{" "}
              by {initiatedBy}
            </>
          }
        />
        {shouldDisplayScheduleLabel(workspace) && (
          <StatsItem
            label={getScheduleLabel(workspace)}
            value={
              <span className={styles.scheduleValue}>
                <Link
                  component={RouterLink}
                  to="settings/schedule"
                  title="Schedule settings"
                >
                  {isWorkspaceOn(workspace)
                    ? autostopDisplay(workspace)
                    : autostartDisplay(workspace.autostart_schedule)}
                </Link>
                {canUpdateWorkspace && canEditDeadline(workspace) && (
                  <span className={styles.scheduleControls}>
                    <IconButton
                      disabled={!deadlineMinusEnabled}
                      size="small"
                      title="Subtract hours from deadline"
                      className={styles.scheduleButton}
                      ref={subButtonRef}
                      onClick={() => setIsSubTime(true)}
                    >
                      <RemoveIcon />
                    </IconButton>
                    <IconButton
                      disabled={!deadlinePlusEnabled}
                      size="small"
                      title="Add hours to deadline"
                      className={styles.scheduleButton}
                      ref={addButtonRef}
                      onClick={() => setIsAddingTime(true)}
                    >
                      <AddIcon />
                    </IconButton>
                  </span>
                )}
              </span>
            }
          />
        )}
        {workspace.latest_build.daily_cost > 0 && (
          <StatsItem
            label={Language.costLabel}
            value={`${workspace.latest_build.daily_cost} ${
              quota_budget ? `/ ${quota_budget}` : ""
            }`}
          />
        )}
      </Stats>

      <Popover
        id="schedule-add"
        classes={{ paper: styles.timePopoverPaper }}
        open={isAddingTime}
        anchorEl={addButtonRef.current}
        onClose={() => setIsAddingTime(false)}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "right",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "right",
        }}
      >
        <span className={styles.timePopoverTitle}>Add hours to deadline</span>
        <span className={styles.timePopoverDescription}>
          Delay the shutdown of this workspace for a few more hours. This is
          only applied once.
        </span>
        <form
          className={styles.timePopoverForm}
          onSubmit={(e) => {
            e.preventDefault()
            const formData = new FormData(e.currentTarget)
            const hours = Number(formData.get("hours"))
            onDeadlinePlus(hours)
            setIsAddingTime(false)
          }}
        >
          <TextField
            name="hours"
            type="number"
            size="small"
            fullWidth
            className={styles.timePopoverField}
            InputProps={{ className: styles.timePopoverFieldInput }}
            inputProps={{
              min: 0,
              max: maxDeadlineIncrease,
              step: 1,
              defaultValue: 1,
            }}
          />

          <Button
            variant="outlined"
            size="small"
            className={styles.timePopoverButton}
            type="submit"
          >
            Apply
          </Button>
        </form>
      </Popover>

      <Popover
        id="schedule-sub"
        classes={{ paper: styles.timePopoverPaper }}
        open={isSubTime}
        anchorEl={subButtonRef.current}
        onClose={() => setIsSubTime(false)}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "right",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "right",
        }}
      >
        <span className={styles.timePopoverTitle}>
          Subtract hours to deadline
        </span>
        <span className={styles.timePopoverDescription}>
          Anticipate the shutdown of this workspace for a few more hours. This
          is only applied once.
        </span>
        <form
          className={styles.timePopoverForm}
          onSubmit={(e) => {
            e.preventDefault()
            const formData = new FormData(e.currentTarget)
            const hours = Number(formData.get("hours"))
            onDeadlineMinus(hours)
            setIsSubTime(false)
          }}
        >
          <TextField
            name="hours"
            type="number"
            size="small"
            fullWidth
            className={styles.timePopoverField}
            InputProps={{ className: styles.timePopoverFieldInput }}
            inputProps={{
              min: 0,
              max: maxDeadlineDecrease,
              step: 1,
              defaultValue: 1,
            }}
          />

          <Button
            variant="outlined"
            size="small"
            className={styles.timePopoverButton}
            type="submit"
          >
            Apply
          </Button>
        </form>
      </Popover>
    </>
  )
}

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

const getScheduleLabel = (workspace: Workspace) => {
  return isWorkspaceOn(workspace) ? "Stops at" : "Starts at"
}

const useStyles = makeStyles((theme) => ({
  scheduleValue: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1.5),
  },

  scheduleControls: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(0.5),
  },

  scheduleButton: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 4,

    "& svg.MuiSvgIcon-root": {
      width: theme.spacing(1.5),
      height: theme.spacing(1.5),
    },
  },

  timePopoverPaper: {
    padding: theme.spacing(3),
    maxWidth: theme.spacing(36),
    marginTop: theme.spacing(1),
    borderRadius: 4,
    display: "flex",
    flexDirection: "column",
    gap: theme.spacing(1),
  },

  timePopoverTitle: {
    fontWeight: 600,
  },

  timePopoverDescription: {
    color: theme.palette.text.secondary,
  },

  timePopoverForm: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1),
    padding: theme.spacing(1, 0),
  },

  timePopoverField: {
    margin: 0,
  },

  timePopoverFieldInput: {
    fontSize: 14,
    padding: theme.spacing(0),
    borderRadius: 4,
  },

  timePopoverButton: {
    borderRadius: 4,
    paddingLeft: theme.spacing(2),
    paddingRight: theme.spacing(2),
    flexShrink: 0,
  },
}))
