import DeleteIcon from "@mui/icons-material/DeleteOutlined";
import DuplicateIcon from "@mui/icons-material/FileCopyOutlined";
import HistoryIcon from "@mui/icons-material/HistoryOutlined";
import MoreVertOutlined from "@mui/icons-material/MoreVertOutlined";
import SettingsIcon from "@mui/icons-material/SettingsOutlined";
import Divider from "@mui/material/Divider";
import { type FC, type ReactNode, Fragment } from "react";
import type { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { TopbarIconButton } from "components/FullPageLayout/Topbar";
import {
  MoreMenu,
  MoreMenuContent,
  MoreMenuItem,
  MoreMenuTrigger,
} from "components/MoreMenu/MoreMenu";
import { useWorkspaceDuplication } from "pages/CreateWorkspacePage/useWorkspaceDuplication";
import { mustUpdateWorkspace } from "utils/workspace";
import {
  CancelButton,
  DisabledButton,
  StartButton,
  StopButton,
  RestartButton,
  UpdateButton,
  ActivateButton,
  FavoriteButton,
} from "./Buttons";
import { type ActionType, abilitiesByWorkspaceStatus } from "./constants";
import { DebugButton } from "./DebugButton";
import { RetryButton } from "./RetryButton";

export interface WorkspaceActionsProps {
  workspace: Workspace;
  handleToggleFavorite: () => void;
  handleStart: (buildParameters?: WorkspaceBuildParameter[]) => void;
  handleStop: () => void;
  handleRestart: (buildParameters?: WorkspaceBuildParameter[]) => void;
  handleDelete: () => void;
  handleUpdate: () => void;
  handleCancel: () => void;
  handleSettings: () => void;
  handleChangeVersion: () => void;
  handleRetry: (buildParameters?: WorkspaceBuildParameter[]) => void;
  handleDebug: (buildParameters?: WorkspaceBuildParameter[]) => void;
  handleDormantActivate: () => void;
  isUpdating: boolean;
  isRestarting: boolean;
  children?: ReactNode;
  canChangeVersions: boolean;
  canDebug: boolean;
  isOwner: boolean;
}

export const WorkspaceActions: FC<WorkspaceActionsProps> = ({
  workspace,
  handleToggleFavorite,
  handleStart,
  handleStop,
  handleRestart,
  handleDelete,
  handleUpdate,
  handleCancel,
  handleSettings,
  handleRetry,
  handleDebug,
  handleChangeVersion,
  handleDormantActivate,
  isUpdating,
  isRestarting,
  canChangeVersions,
  canDebug,
  isOwner,
}) => {
  const { duplicateWorkspace, isDuplicationReady } =
    useWorkspaceDuplication(workspace);

  const { actions, canCancel, canAcceptJobs } = abilitiesByWorkspaceStatus(
    workspace,
    canDebug,
  );
  const showCancel =
    canCancel &&
    (workspace.template_allow_user_cancel_workspace_jobs || isOwner);

  const mustUpdate = mustUpdateWorkspace(workspace, canChangeVersions);
  const tooltipText = getTooltipText(workspace, mustUpdate, canChangeVersions);
  const canBeUpdated = workspace.outdated && canAcceptJobs;

  // A mapping of button type to the corresponding React component
  const buttonMapping: Record<ActionType, ReactNode> = {
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
    deleting: <DisabledButton label="Deleting" />,
    canceling: <DisabledButton label="Canceling..." />,
    deleted: <DisabledButton label="Deleted" />,
    pending: <DisabledButton label="Pending..." />,
    activate: <ActivateButton handleAction={handleDormantActivate} />,
    activating: <ActivateButton loading handleAction={handleDormantActivate} />,
    retry: (
      <RetryButton
        handleAction={handleRetry}
        workspace={workspace}
        enableBuildParameters={workspace.latest_build.transition === "start"}
      />
    ),
    debug: (
      <DebugButton
        handleAction={handleDebug}
        workspace={workspace}
        enableBuildParameters={workspace.latest_build.transition === "start"}
      />
    ),
    toggleFavorite: (
      <FavoriteButton
        workspaceID={workspace.id}
        isFavorite={workspace.favorite}
        onToggle={handleToggleFavorite}
      />
    ),
  };

  return (
    <div
      css={{ display: "flex", alignItems: "center", gap: 8 }}
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

      {showCancel && <CancelButton handleAction={handleCancel} />}

      {buttonMapping.toggleFavorite}

      <MoreMenu>
        <MoreMenuTrigger>
          <TopbarIconButton
            title="More options"
            data-testid="workspace-options-button"
            aria-controls="workspace-options"
            disabled={!canAcceptJobs}
          >
            <MoreVertOutlined />
          </TopbarIconButton>
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

function getTooltipText(
  workspace: Workspace,
  mustUpdate: boolean,
  canChangeVersions: boolean,
): string {
  if (!mustUpdate && !canChangeVersions) {
    return "";
  }

  if (!mustUpdate && canChangeVersions) {
    return "This template requires automatic updates on workspace startup, but template administrators can ignore this policy.";
  }

  if (workspace.template_require_active_version) {
    return "This template requires automatic updates on workspace startup. Contact your administrator if you want to preserve the template version.";
  }

  if (workspace.automatic_updates === "always") {
    return "Automatic updates are enabled for this workspace. Modify the update policy in workspace settings if you want to preserve the template version.";
  }

  return "";
}
