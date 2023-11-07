import { FC, Fragment, ReactNode } from "react";
import { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { useWorkspaceDuplication } from "pages/CreateWorkspacePage/useWorkspaceDuplication";
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

import Divider from "@mui/material/Divider";
import DuplicateIcon from "@mui/icons-material/FileCopyOutlined";
import SettingsIcon from "@mui/icons-material/SettingsOutlined";
import HistoryIcon from "@mui/icons-material/HistoryOutlined";
import DeleteIcon from "@mui/icons-material/DeleteOutlined";

import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
  ThreeDotsButton,
} from "components/MoreMenu/MoreMenu";

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
  const { duplicateWorkspace, isDuplicationReady } =
    useWorkspaceDuplication(workspace);

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
      css={{ display: "flex", alignItems: "center", gap: 12 }}
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
        <MoreMenuTrigger>
          <ThreeDotsButton
            title="More options"
            size="small"
            data-testid="workspace-options-button"
            aria-controls="workspace-options"
            disabled={!canAcceptJobs}
          />
        </MoreMenuTrigger>

        <MoreMenuContent id="workspace-options">
          <MoreMenuItem onClick={handleSettings}>
            <SettingsIcon />
            Settings
          </MoreMenuItem>

          {canChangeVersions && (
            <MoreMenuItem onClick={handleChangeVersion}>
              <HistoryIcon />
              Change version&hellip;
            </MoreMenuItem>
          )}

          <MoreMenuItem
            onClick={duplicateWorkspace}
            disabled={!isDuplicationReady}
          >
            <DuplicateIcon />
            Duplicate&hellip;
          </MoreMenuItem>

          <Divider />

          <MoreMenuItem
            danger
            onClick={handleDelete}
            data-testid="delete-button"
          >
            <DeleteIcon />
            Delete&hellip;
          </MoreMenuItem>
        </MoreMenuContent>
      </MoreMenu>
    </div>
  );
};
