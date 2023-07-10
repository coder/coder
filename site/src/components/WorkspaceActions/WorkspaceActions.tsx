import MenuItem from "@mui/material/MenuItem"
import Menu from "@mui/material/Menu"
import { makeStyles } from "@mui/styles"
import MoreVertOutlined from "@mui/icons-material/MoreVertOutlined"
import { FC, Fragment, ReactNode, useRef, useState } from "react"
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
import Divider from "@mui/material/Divider"
import VisibilityOffOutlined from "@mui/icons-material/VisibilityOffOutlined"
import VisibilityOutlined from "@mui/icons-material/VisibilityOutlined"
import { useLocalPreferences } from "contexts/LocalPreferencesContext"

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
  canChangeBuildLogsVisibility: boolean
  isWorkspaceBuildLogsUIActive: boolean
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
  canChangeBuildLogsVisibility,
  isWorkspaceBuildLogsUIActive,
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
  const localPreferences = useLocalPreferences()
  const isBuildLogsVisible =
    localPreferences.getPreference("buildLogsVisibility") === "visible"

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
          <Fragment key={action}>{buttonMapping[action]}</Fragment>
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

          {isWorkspaceBuildLogsUIActive && (
            <>
              <Divider sx={{ borderColor: (theme) => theme.palette.divider }} />
              {isBuildLogsVisible ? (
                <MenuItem
                  disabled={!canChangeBuildLogsVisibility}
                  onClick={onMenuItemClick(() => {
                    localPreferences.setPreference(
                      "buildLogsVisibility",
                      "hide",
                    )
                  })}
                >
                  <VisibilityOffOutlined />
                  Hide build logs
                </MenuItem>
              ) : (
                <MenuItem
                  disabled={!canChangeBuildLogsVisibility}
                  onClick={onMenuItemClick(() => {
                    localPreferences.setPreference(
                      "buildLogsVisibility",
                      "visible",
                    )
                  })}
                >
                  <VisibilityOutlined />
                  Show build logs
                </MenuItem>
              )}
            </>
          )}
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
