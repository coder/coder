import MenuItem from "@material-ui/core/MenuItem"
import Button from "@material-ui/core/Button"
import Menu from "@material-ui/core/Menu"
import { makeStyles } from "@material-ui/core/styles"
import MoreVertOutlined from "@material-ui/icons/MoreVertOutlined"
import { FC, ReactNode, useRef, useState } from "react"
import { useTranslation } from "react-i18next"
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
import SettingsOutlined from "@material-ui/icons/SettingsOutlined"
import HistoryOutlined from "@material-ui/icons/HistoryOutlined"
import DeleteOutlined from "@material-ui/icons/DeleteOutlined"

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
  const { t } = useTranslation("workspacePage")
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
    [ButtonTypesEnum.update]: (
      <UpdateButton handleAction={handleUpdate} key={ButtonTypesEnum.update} />
    ),
    [ButtonTypesEnum.updating]: (
      <ActionLoadingButton
        label={t("actionButton.updating")}
        key={ButtonTypesEnum.updating}
      />
    ),
    [ButtonTypesEnum.start]: (
      <StartButton handleAction={handleStart} key={ButtonTypesEnum.start} />
    ),
    [ButtonTypesEnum.starting]: (
      <ActionLoadingButton
        label={t("actionButton.starting")}
        key={ButtonTypesEnum.starting}
      />
    ),
    [ButtonTypesEnum.stop]: (
      <StopButton handleAction={handleStop} key={ButtonTypesEnum.stop} />
    ),
    [ButtonTypesEnum.stopping]: (
      <ActionLoadingButton
        label={t("actionButton.stopping")}
        key={ButtonTypesEnum.stopping}
      />
    ),
    [ButtonTypesEnum.restart]: <RestartButton handleAction={handleRestart} />,
    [ButtonTypesEnum.restarting]: (
      <ActionLoadingButton
        label="Restarting"
        key={ButtonTypesEnum.restarting}
      />
    ),
    [ButtonTypesEnum.deleting]: (
      <ActionLoadingButton
        label={t("actionButton.deleting")}
        key={ButtonTypesEnum.deleting}
      />
    ),
    [ButtonTypesEnum.canceling]: (
      <DisabledButton
        label={t("disabledButton.canceling")}
        key={ButtonTypesEnum.canceling}
      />
    ),
    [ButtonTypesEnum.deleted]: (
      <DisabledButton
        label={t("disabledButton.deleted")}
        key={ButtonTypesEnum.deleted}
      />
    ),
    [ButtonTypesEnum.pending]: (
      <ActionLoadingButton
        label={t("disabledButton.pending")}
        key={ButtonTypesEnum.pending}
      />
    ),
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
        <Button
          data-testid="workspace-options-button"
          aria-controls="workspace-options"
          aria-haspopup="true"
          variant="outlined"
          disabled={!canAcceptJobs}
          ref={menuTriggerRef}
          onClick={() => setIsMenuOpen(true)}
        >
          <MoreVertOutlined />
        </Button>
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
    gap: theme.spacing(2),
  },
}))
