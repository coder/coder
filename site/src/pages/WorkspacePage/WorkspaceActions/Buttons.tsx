import { type FC } from "react";
import type { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { BuildParametersPopover } from "./BuildParametersPopover";

import Tooltip from "@mui/material/Tooltip";
import Button from "@mui/material/Button";
import LoadingButton from "@mui/lab/LoadingButton";
import ButtonGroup from "@mui/material/ButtonGroup";
import CloudQueueIcon from "@mui/icons-material/CloudQueue";
import CropSquareIcon from "@mui/icons-material/CropSquare";
import PlayCircleOutlineIcon from "@mui/icons-material/PlayCircleOutline";
import ReplayIcon from "@mui/icons-material/Replay";
import BlockIcon from "@mui/icons-material/Block";
import OutlinedBlockIcon from "@mui/icons-material/BlockOutlined";
import PowerSettingsNewIcon from "@mui/icons-material/PowerSettingsNew";
import RetryIcon from "@mui/icons-material/BuildOutlined";
import RetryDebugIcon from "@mui/icons-material/BugReportOutlined";

interface WorkspaceActionProps {
  loading?: boolean;
  handleAction: () => void;
  disabled?: boolean;
  tooltipText?: string;
}

export const UpdateButton: FC<WorkspaceActionProps> = ({
  handleAction,
  loading,
}) => {
  return (
    <LoadingButton
      loading={loading}
      loadingPosition="start"
      data-testid="workspace-update-button"
      startIcon={<CloudQueueIcon />}
      onClick={handleAction}
    >
      {loading ? <>Updating&hellip;</> : <>Update&hellip;</>}
    </LoadingButton>
  );
};

export const ActivateButton: FC<WorkspaceActionProps> = ({
  handleAction,
  loading,
}) => {
  return (
    <LoadingButton
      loading={loading}
      loadingPosition="start"
      startIcon={<PowerSettingsNewIcon />}
      onClick={handleAction}
    >
      {loading ? <>Activating&hellip;</> : "Activate"}
    </LoadingButton>
  );
};

type StartButtonProps = Omit<WorkspaceActionProps, "handleAction"> & {
  workspace: Workspace;
  handleAction: (buildParameters?: WorkspaceBuildParameter[]) => void;
};

export const StartButton: FC<StartButtonProps> = ({
  handleAction,
  workspace,
  loading,
  disabled,
  tooltipText,
}) => {
  return (
    <ButtonGroup
      variant="outlined"
      css={{
        // Workaround to make the border transitions smmothly on button groups
        "& > button:hover + button": {
          borderLeft: "1px solid #FFF",
        },
      }}
      disabled={disabled}
    >
      <LoadingButton
        loading={loading}
        loadingPosition="start"
        startIcon={<PlayCircleOutlineIcon />}
        onClick={() => handleAction()}
        disabled={disabled}
      >
        {loading ? <>Starting&hellip;</> : "Start"}
      </LoadingButton>
      <BuildParametersPopover
        workspace={workspace}
        disabled={loading}
        onSubmit={handleAction}
      />
    </ButtonGroup>
  );
};

export const StopButton: FC<WorkspaceActionProps> = ({
  handleAction,
  loading,
}) => {
  return (
    <LoadingButton
      loading={loading}
      loadingPosition="start"
      startIcon={<CropSquareIcon />}
      onClick={handleAction}
      data-testid="workspace-stop-button"
    >
      {loading ? <>Stopping&hellip;</> : "Stop"}
    </LoadingButton>
  );
};

type RestartButtonProps = Omit<WorkspaceActionProps, "handleAction"> & {
  workspace: Workspace;
  handleAction: (buildParameters?: WorkspaceBuildParameter[]) => void;
};

export const RestartButton: FC<RestartButtonProps> = ({
  handleAction,
  loading,
  workspace,
  disabled,
  tooltipText,
}) => {
  return (
    <ButtonGroup
      variant="outlined"
      css={{
        // Workaround to make the border transitions smmothly on button groups
        "& > button:hover + button": {
          borderLeft: "1px solid #FFF",
        },
      }}
      disabled={disabled}
    >
      <LoadingButton
        loading={loading}
        loadingPosition="start"
        startIcon={<ReplayIcon />}
        onClick={() => handleAction()}
        data-testid="workspace-restart-button"
        disabled={disabled}
      >
        {loading ? <>Restarting&hellip;</> : <>Restart&hellip;</>}
      </LoadingButton>
      <BuildParametersPopover
        workspace={workspace}
        disabled={loading}
        onSubmit={handleAction}
      />
    </ButtonGroup>
  );
};

export const CancelButton: FC<WorkspaceActionProps> = ({ handleAction }) => {
  return (
    <Button startIcon={<BlockIcon />} onClick={handleAction}>
      Cancel
    </Button>
  );
};

interface DisabledProps {
  label: string;
}

export const DisabledButton: FC<DisabledProps> = ({ label }) => {
  return (
    <Button startIcon={<OutlinedBlockIcon />} disabled>
      {label}
    </Button>
  );
};

interface LoadingProps {
  label: string;
}

export const ActionLoadingButton: FC<LoadingProps> = ({ label }) => {
  return (
    <LoadingButton loading loadingPosition="start" startIcon={<ReplayIcon />}>
      {label}
    </LoadingButton>
  );
};

type RetryButtonProps = Omit<WorkspaceActionProps, "loading"> & {
  debug?: boolean;
};

export const RetryButton: FC<RetryButtonProps> = ({
  handleAction,
  debug = false,
}) => {
  return (
    <Button
      startIcon={debug ? <RetryDebugIcon /> : <RetryIcon />}
      onClick={handleAction}
    >
      Retry{debug && " (Debug)"}
    </Button>
  );
};
