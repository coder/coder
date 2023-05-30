import MenuItem from "@mui/material/MenuItem"
import Menu from "@mui/material/Menu"
import { makeStyles } from "@mui/styles"
import MoreVertOutlined from "@mui/icons-material/MoreVertOutlined"
import { FC, ReactNode, useRef, useState } from "react"
import { WorkspaceStatus } from "api/typesGenerated"
import {
  ActionLoadingButton,
  CancelButton,
  DisabledButton,
  StartButton,
  StopButton,
  RestartButton,
  UpdateButton,
} from "./Buttons"
import {
  ButtonMapping,
  ButtonTypesEnum,
  actionsByWorkspaceStatus,
} from "./constants"
import SettingsOutlined from "@mui/icons-material/SettingsOutlined"
import HistoryOutlined from "@mui/icons-material/HistoryOutlined"
import DeleteOutlined from "@mui/icons-material/DeleteOutlined"
import IconButton from "@mui/material/IconButton"

export interface WorkspaceActionsProps {
  workspaceStatus: WorkspaceStatus
  isOutdated: boolean
  handleStart: () => void
  handleStop: () => void
  handleRestart: () => void
  handleDelete: () => void
  handleUpdate: () => void
  handleCancel: () => void
  handleSettings: () => void
  handleChangeVersion: () => void
  isUpdating: boolean
  isRestarting: boolean
  children?: ReactNode
  canChangeVersions: boolean
}

export const WorkspaceActions: FC<WorkspaceActionsProps> = ({
  workspaceStatus,
  isOutdated,
  handleStart,
  handleStop,
  handleRestart,
  handleDelete,
  handleUpdate,
  handleCancel,
  handleSettings,
  handleChangeVersion,
  isUpdating,
  isRestarting,
  canChangeVersions,
}) => {
  const styles = useStyles()
  const {
    canCancel,
    canAcceptJobs,
    actions: actionsByStatus,
  } = actionsByWorkspaceStatus(workspaceStatus)
  const canBeUpdated = isOutdated && canAcceptJobs
  const menuTriggerRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)

  // A mapping of button type to the corresponding React component
  const buttonMapping: ButtonMapping = {
    [ButtonTypesEnum.update]: <UpdateButton handleAction={handleUpdate} />,
    [ButtonTypesEnum.updating]: (
      <UpdateButton loading handleAction={handleUpdate} />
    ),
    [ButtonTypesEnum.start]: <StartButton handleAction={handleStart} />,
    [ButtonTypesEnum.starting]: (
      <StartButton loading handleAction={handleStart} />
    ),
    [ButtonTypesEnum.stop]: <StopButton handleAction={handleStop} />,
    [ButtonTypesEnum.stopping]: (
      <StopButton loading handleAction={handleStop} />
    ),
    [ButtonTypesEnum.restart]: <RestartButton handleAction={handleRestart} />,
    [ButtonTypesEnum.restarting]: (
      <RestartButton loading handleAction={handleRestart} />
    ),
    [ButtonTypesEnum.deleting]: <ActionLoadingButton label="Deleting" />,
    [ButtonTypesEnum.canceling]: <DisabledButton label="Canceling..." />,
    [ButtonTypesEnum.deleted]: <DisabledButton label="Deleted" />,
    [ButtonTypesEnum.pending]: <ActionLoadingButton label="Pending..." />,
  }

  // Returns a function that will execute the action and close the menu
  const onMenuItemClick = (actionFn: () => void) => () => {
    setIsMenuOpen(false)
    actionFn()
  }

  return (
    <div className={styles.actions} data-testid="workspace-actions">
      {canBeUpdated &&
        (isUpdating
          ? buttonMapping[ButtonTypesEnum.updating]
          : buttonMapping[ButtonTypesEnum.update])}
      {isRestarting && buttonMapping[ButtonTypesEnum.restarting]}
      {!isRestarting &&
        actionsByStatus.map((action) => (
          <span key={action}>{buttonMapping[action]}</span>
        ))}
      {canCancel && <CancelButton handleAction={handleCancel} />}
      <div>
        <IconButton
          title="More options"
          size="small"
          data-testid="workspace-options-button"
          aria-controls="workspace-options"
          aria-haspopup="true"
          disabled={!canAcceptJobs}
          ref={menuTriggerRef}
          onClick={() => setIsMenuOpen(true)}
        >
          <MoreVertOutlined />
        </IconButton>
        <Menu
          id="workspace-options"
          anchorEl={menuTriggerRef.current}
          open={isMenuOpen}
          onClose={() => setIsMenuOpen(false)}
        >
          <MenuItem onClick={onMenuItemClick(handleSettings)}>
            <SettingsOutlined />
            Settings
          </MenuItem>
          {canChangeVersions && (
            <MenuItem onClick={onMenuItemClick(handleChangeVersion)}>
              <HistoryOutlined />
              Change version
            </MenuItem>
          )}
          <MenuItem onClick={onMenuItemClick(handleDelete)}>
            <DeleteOutlined />
            Delete
          </MenuItem>
        </Menu>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  actions: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1.5),
  },
}))
