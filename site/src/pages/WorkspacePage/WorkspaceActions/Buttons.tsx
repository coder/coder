import Button from "@mui/material/Button";
import BlockIcon from "@mui/icons-material/Block";
import CloudQueueIcon from "@mui/icons-material/CloudQueue";
import CropSquareIcon from "@mui/icons-material/CropSquare";
import PlayCircleOutlineIcon from "@mui/icons-material/PlayCircleOutline";
import ReplayIcon from "@mui/icons-material/Replay";
import { FC } from "react";
import BlockOutlined from "@mui/icons-material/BlockOutlined";
import ButtonGroup from "@mui/material/ButtonGroup";
import { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { BuildParametersPopover } from "./BuildParametersPopover";
import PowerSettingsNewIcon from "@mui/icons-material/PowerSettingsNew";
import LoadingButton from "@mui/lab/LoadingButton";
import Tooltip from "@mui/material/Tooltip";

interface WorkspaceAction {
  loading?: boolean;
  handleAction: () => void;
  disabled?: boolean;
  tooltipText?: string;
}

export const UpdateButton: FC<WorkspaceAction> = ({
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

export const ActivateButton: FC<WorkspaceAction> = ({
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

export const StartButton: FC<
  Omit<WorkspaceAction, "handleAction"> & {
    workspace: Workspace;
    handleAction: (buildParameters?: WorkspaceBuildParameter[]) => void;
  }
> = ({ handleAction, workspace, loading, disabled, tooltipText }) => {
  const buttonContent = (
    <ButtonGroup
      variant="outlined"
      sx={{
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

  return tooltipText ? (
    <Tooltip title={tooltipText}>{buttonContent}</Tooltip>
  ) : (
    buttonContent
  );
};

export const StopButton: FC<WorkspaceAction> = ({ handleAction, loading }) => {
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

export const RestartButton: FC<
  Omit<WorkspaceAction, "handleAction"> & {
    workspace: Workspace;
    handleAction: (buildParameters?: WorkspaceBuildParameter[]) => void;
  }
> = ({ handleAction, loading, workspace, disabled, tooltipText }) => {
  const buttonContent = (
    <ButtonGroup
      variant="outlined"
      sx={{
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

  return tooltipText ? (
    <Tooltip title={tooltipText}>{buttonContent}</Tooltip>
  ) : (
    buttonContent
  );
};

export const CancelButton: FC<WorkspaceAction> = ({ handleAction }) => {
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
    <Button startIcon={<BlockOutlined />} disabled>
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
