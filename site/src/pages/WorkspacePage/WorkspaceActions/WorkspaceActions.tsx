import { type FC, type ReactNode, Fragment } from "react";
import { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { useWorkspaceDuplication } from "pages/CreateWorkspacePage/useWorkspaceDuplication";

import { workspaceUpdatePolicy } from "utils/workspace";
import { type ButtonType, actionsByWorkspaceStatus } from "./constants";

import {
  ActionLoadingButton,
  CancelButton,
  DisabledButton,
  StartButton,
  StopButton,
  RestartButton,
  UpdateButton,
  ActivateButton,
  RetryButton,
} from "./Buttons";

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
  handleRetry: () => void;
  handleRetryDebug: () => void;
  handleDormantActivate: () => void;
  isUpdating: boolean;
  isRestarting: boolean;
  children?: ReactNode;
  canChangeVersions: boolean;
  canRetryDebug: boolean;
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
  handleRetry,
  handleRetryDebug,
  handleChangeVersion,
  handleDormantActivate,
  isUpdating,
  isRestarting,
  canChangeVersions,
  canRetryDebug,
}) => {
  const { duplicateWorkspace, isDuplicationReady } =
    useWorkspaceDuplication(workspace);

  const { actions, canCancel, canAcceptJobs } = actionsByWorkspaceStatus(
    workspace,
    canRetryDebug,
  );

  const mustUpdate =
    workspaceUpdatePolicy(workspace, canChangeVersions) === "always" &&
    workspace.outdated;

  const tooltipText = getTooltipText(workspace, mustUpdate);
  const canBeUpdated = workspace.outdated && canAcceptJobs;

  // A mapping of button type to the corresponding React component
  const buttonMapping: Record<ButtonType, ReactNode> = {
    update: <UpdateButton handleAction={handleUpdate} />,
    updating: <UpdateButton loading handleAction={handleUpdate} />,
    start: (
      <StartButton
        workspace={workspace}
        handleAction={handleStart}
        disabled={mustUpdate}
        tooltipText={tooltipText}
      />
    ),
    starting: (
      <StartButton
        loading
        workspace={workspace}
        handleAction={handleStart}
        disabled={mustUpdate}
        tooltipText={tooltipText}
      />
    ),
    stop: <StopButton handleAction={handleStop} />,
    stopping: <StopButton loading handleAction={handleStop} />,
    restart: (
      <RestartButton
        workspace={workspace}
        handleAction={handleRestart}
        disabled={mustUpdate}
        tooltipText={tooltipText}
      />
    ),
    restarting: (
      <RestartButton
        loading
        workspace={workspace}
        handleAction={handleRestart}
        disabled={mustUpdate}
        tooltipText={tooltipText}
      />
    ),
    deleting: <ActionLoadingButton label="Deleting" />,
    canceling: <DisabledButton label="Canceling..." />,
    deleted: <DisabledButton label="Deleted" />,
    pending: <ActionLoadingButton label="Pending..." />,
    activate: <ActivateButton handleAction={handleDormantActivate} />,
    activating: <ActivateButton loading handleAction={handleDormantActivate} />,
    retry: <RetryButton handleAction={handleRetry} />,
    retryDebug: <RetryButton debug handleAction={handleRetryDebug} />,
  };

  return (
    <div
      css={{ display: "flex", alignItems: "center", gap: 12 }}
      data-testid="workspace-actions"
    >
      {canBeUpdated && (
        <>{isUpdating ? buttonMapping.updating : buttonMapping.update}</>
      )}

      {isRestarting
        ? buttonMapping.restarting
        : actions.map((action) => (
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

function getTooltipText(workspace: Workspace, disabled: boolean): string {
  if (!disabled) {
    return "";
  }

  if (workspace.template_require_active_version) {
    return "This template requires automatic updates";
  }

  if (workspace.automatic_updates === "always") {
    return "You have enabled automatic updates for this workspace";
  }

  return "";
}
