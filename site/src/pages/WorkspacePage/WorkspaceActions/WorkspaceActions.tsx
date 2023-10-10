import { type FC, Fragment, type ReactNode, useRef, useState } from "react";
import { ButtonMapping, actionsByWorkspaceStatus } from "./constants";
import {
  type Workspace,
  type WorkspaceBuildParameter,
} from "api/typesGenerated";

import {
  ActionLoadingButton,
  CancelButton,
  DisabledButton,
  StartButton,
  StopButton,
  RestartButton,
  UpdateButton,
  ActivateButton,
} from "./Buttons";

import { makeStyles } from "@mui/styles";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import IconButton from "@mui/material/IconButton";

import SettingsIcon from "@mui/icons-material/SettingsOutlined";
import HistoryIcon from "@mui/icons-material/HistoryOutlined";
import DeleteIcon from "@mui/icons-material/DeleteOutlined";
import MoreOptionsIcon from "@mui/icons-material/MoreVertOutlined";
import DuplicateIcon from "@mui/icons-material/FileCopyOutlined";

export interface WorkspaceActionsProps {
  workspace: Workspace;
  handleStart: (buildParameters?: WorkspaceBuildParameter[]) => void;
  handleStop: () => void;
  handleRestart: (buildParameters?: WorkspaceBuildParameter[]) => void;
  handleDelete: () => void;
  handleUpdate: () => void;
  handleCancel: () => void;
  handleClone: () => void;
  handleSettings: () => void;
  handleChangeVersion: () => void;
  handleDormantActivate: () => void;
  isUpdating: boolean;
  isRestarting: boolean;
  children?: ReactNode;
  canChangeVersions: boolean;
}

export const WorkspaceActions: FC<WorkspaceActionsProps> = ({
  workspace,
  handleStart,
  handleStop,
  handleRestart,
  handleDelete,
  handleUpdate,
  handleCancel,
  handleSettings,
  handleClone,
  handleChangeVersion,
  handleDormantActivate: handleDormantActivate,
  isUpdating,
  isRestarting,
  canChangeVersions,
}) => {
  // A mapping of button type to the corresponding React component
  const buttonMapping: ButtonMapping = {
    update: <UpdateButton handleAction={handleUpdate} />,
    updating: <UpdateButton loading handleAction={handleUpdate} />,
    start: <StartButton workspace={workspace} handleAction={handleStart} />,
    starting: (
      <StartButton loading workspace={workspace} handleAction={handleStart} />
    ),
    stop: <StopButton handleAction={handleStop} />,
    stopping: <StopButton loading handleAction={handleStop} />,
    restart: (
      <RestartButton workspace={workspace} handleAction={handleRestart} />
    ),
    restarting: (
      <RestartButton
        loading
        workspace={workspace}
        handleAction={handleRestart}
      />
    ),
    deleting: <ActionLoadingButton label="Deleting" />,
    canceling: <DisabledButton label="Canceling..." />,
    deleted: <DisabledButton label="Deleted" />,
    pending: <ActionLoadingButton label="Pending..." />,
    activate: <ActivateButton handleAction={handleDormantActivate} />,
    activating: <ActivateButton loading handleAction={handleDormantActivate} />,
  };

  const styles = useStyles();
  const menuTriggerRef = useRef<HTMLButtonElement>(null);
  const [isMenuOpen, setIsMenuOpen] = useState(false);

  // Returns a function that will execute the action and close the menu
  const onMenuItemClick = (actionFn: () => void) => () => {
    setIsMenuOpen(false);
    actionFn();
  };

  const {
    canCancel,
    canAcceptJobs,
    actions: actionsByStatus,
  } = actionsByWorkspaceStatus(workspace, workspace.latest_build.status);
  const canBeUpdated = workspace.outdated && canAcceptJobs;

  return (
    <div className={styles.actions} data-testid="workspace-actions">
      {canBeUpdated &&
        (isUpdating ? buttonMapping.updating : buttonMapping.update)}

      {isRestarting && buttonMapping.restarting}

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
          <MoreOptionsIcon />
        </IconButton>

        <Menu
          id="workspace-options"
          anchorEl={menuTriggerRef.current}
          open={isMenuOpen}
          onClose={() => setIsMenuOpen(false)}
        >
          <MenuItem onClick={onMenuItemClick(handleSettings)}>
            <SettingsIcon />
            Settings
          </MenuItem>

          {canChangeVersions && (
            <MenuItem onClick={onMenuItemClick(handleChangeVersion)}>
              <HistoryIcon />
              Change version&hellip;
            </MenuItem>
          )}

          <MenuItem onClick={onMenuItemClick(handleClone)}>
            <DuplicateIcon />
            Duplicate&hellip;
          </MenuItem>

          <MenuItem
            onClick={onMenuItemClick(handleDelete)}
            data-testid="delete-button"
          >
            <DeleteIcon />
            Delete&hellip;
          </MenuItem>
        </Menu>
      </div>
    </div>
  );
};

const useStyles = makeStyles((theme) => ({
  actions: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1.5),
  },
}));
