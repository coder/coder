import { FC, Fragment, ReactNode } from "react";
import { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
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
import {
  ButtonMapping,
  ButtonTypesEnum,
  actionsByWorkspaceStatus,
} from "./constants";
import SettingsOutlined from "@mui/icons-material/SettingsOutlined";
import HistoryOutlined from "@mui/icons-material/HistoryOutlined";
import DeleteOutlined from "@mui/icons-material/DeleteOutlined";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
} from "components/MoreMenu/MoreMenu";
import Divider from "@mui/material/Divider";

export interface WorkspaceActionsProps {
  workspace: Workspace;
  handleStart: (buildParameters?: WorkspaceBuildParameter[]) => void;
  handleStop: () => void;
  handleRestart: (buildParameters?: WorkspaceBuildParameter[]) => void;
  handleDelete: () => void;
  handleUpdate: () => void;
  handleCancel: () => void;
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
  handleChangeVersion,
  handleDormantActivate: handleDormantActivate,
  isUpdating,
  isRestarting,
  canChangeVersions,
}) => {
  const {
    canCancel,
    canAcceptJobs,
    actions: actionsByStatus,
  } = actionsByWorkspaceStatus(
    workspace,
    workspace.latest_build.status,
    canChangeVersions,
  );
  const canBeUpdated = workspace.outdated && canAcceptJobs;

  // A mapping of button type to the corresponding React component
  const buttonMapping: ButtonMapping = {
    [ButtonTypesEnum.update]: <UpdateButton handleAction={handleUpdate} />,
    [ButtonTypesEnum.updating]: (
      <UpdateButton loading handleAction={handleUpdate} />
    ),
    [ButtonTypesEnum.start]: (
      <StartButton workspace={workspace} handleAction={handleStart} />
    ),
    [ButtonTypesEnum.starting]: (
      <StartButton loading workspace={workspace} handleAction={handleStart} />
    ),
    [ButtonTypesEnum.stop]: <StopButton handleAction={handleStop} />,
    [ButtonTypesEnum.stopping]: (
      <StopButton loading handleAction={handleStop} />
    ),
    [ButtonTypesEnum.restart]: (
      <RestartButton workspace={workspace} handleAction={handleRestart} />
    ),
    [ButtonTypesEnum.restarting]: (
      <RestartButton
        loading
        workspace={workspace}
        handleAction={handleRestart}
      />
    ),
    [ButtonTypesEnum.deleting]: <ActionLoadingButton label="Deleting" />,
    [ButtonTypesEnum.canceling]: <DisabledButton label="Canceling..." />,
    [ButtonTypesEnum.deleted]: <DisabledButton label="Deleted" />,
    [ButtonTypesEnum.pending]: <ActionLoadingButton label="Pending..." />,
    [ButtonTypesEnum.activate]: (
      <ActivateButton handleAction={handleDormantActivate} />
    ),
    [ButtonTypesEnum.activating]: (
      <ActivateButton loading handleAction={handleDormantActivate} />
    ),
  };

  return (
    <div
      css={(theme) => ({
        display: "flex",
        alignItems: "center",
        gap: theme.spacing(1.5),
      })}
      data-testid="workspace-actions"
    >
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
      <MoreMenu>
        <MoreMenuTrigger
          title="More options"
          size="small"
          data-testid="workspace-options-button"
          aria-controls="workspace-options"
          disabled={!canAcceptJobs}
        />
        <MoreMenuContent id="workspace-options">
          <MoreMenuItem onClick={handleSettings}>
            <SettingsOutlined />
            Settings
          </MoreMenuItem>
          {canChangeVersions && (
            <MoreMenuItem onClick={handleChangeVersion}>
              <HistoryOutlined />
              Change version&hellip;
            </MoreMenuItem>
          )}
          <Divider />
          <MoreMenuItem
            danger
            onClick={handleDelete}
            data-testid="delete-button"
          >
            <DeleteOutlined />
            Delete&hellip;
          </MoreMenuItem>
        </MoreMenuContent>
      </MoreMenu>
    </div>
  );
};
