import Button from "@mui/material/Button";
import BlockIcon from "@mui/icons-material/Block";
import CloudQueueIcon from "@mui/icons-material/CloudQueue";
import CropSquareIcon from "@mui/icons-material/CropSquare";
import PlayCircleOutlineIcon from "@mui/icons-material/PlayCircleOutline";
import ReplayIcon from "@mui/icons-material/Replay";
import { LoadingButton } from "components/LoadingButton/LoadingButton";
import { FC } from "react";
import BlockOutlined from "@mui/icons-material/BlockOutlined";
import ButtonGroup from "@mui/material/ButtonGroup";
import { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { BuildParametersPopover } from "./BuildParametersPopover";
import PowerSettingsNewIcon from "@mui/icons-material/PowerSettingsNew";

interface WorkspaceAction {
  loading?: boolean;
  handleAction: () => void;
}

export const UpdateButton: FC<WorkspaceAction> = ({
  handleAction,
  loading,
}) => {
  return (
    <LoadingButton
      loading={loading}
      loadingIndicator="Updating..."
      loadingPosition="start"
      data-testid="workspace-update-button"
      startIcon={<CloudQueueIcon />}
      onClick={handleAction}
    >
      Update
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
      loadingIndicator="Activating..."
      loadingPosition="start"
      startIcon={<PowerSettingsNewIcon />}
      onClick={handleAction}
    >
      Activate
    </LoadingButton>
  );
};

export const StartButton: FC<
  Omit<WorkspaceAction, "handleAction"> & {
    workspace: Workspace;
    handleAction: (buildParameters?: WorkspaceBuildParameter[]) => void;
  }
> = ({ handleAction, workspace, loading }) => {
  return (
    <ButtonGroup
      variant="outlined"
      sx={{
        // Workaround to make the border transitions smmothly on button groups
        "& > button:hover + button": {
          borderLeft: "1px solid #FFF",
        },
      }}
    >
      <LoadingButton
        loading={loading}
        loadingIndicator="Starting..."
        loadingPosition="start"
        startIcon={<PlayCircleOutlineIcon />}
        onClick={() => handleAction()}
      >
        Start
      </LoadingButton>
      <BuildParametersPopover
        workspace={workspace}
        disabled={loading}
        onSubmit={handleAction}
      />
    </ButtonGroup>
  );
};

export const StopButton: FC<WorkspaceAction> = ({ handleAction, loading }) => {
  return (
    <LoadingButton
      loading={loading}
      loadingIndicator="Stopping..."
      loadingPosition="start"
      startIcon={<CropSquareIcon />}
      onClick={handleAction}
      data-testid="workspace-stop-button"
    >
      Stop
    </LoadingButton>
  );
};

export const RestartButton: FC<
  Omit<WorkspaceAction, "handleAction"> & {
    workspace: Workspace;
    handleAction: (buildParameters?: WorkspaceBuildParameter[]) => void;
  }
> = ({ handleAction, loading, workspace }) => {
  return (
    <ButtonGroup
      variant="outlined"
      sx={{
        // Workaround to make the border transitions smmothly on button groups
        "& > button:hover + button": {
          borderLeft: "1px solid #FFF",
        },
      }}
    >
      <LoadingButton
        loading={loading}
        loadingIndicator="Restarting..."
        loadingPosition="start"
        startIcon={<ReplayIcon />}
        onClick={() => handleAction()}
        data-testid="workspace-restart-button"
      >
        Restart
      </LoadingButton>
      <BuildParametersPopover
        workspace={workspace}
        disabled={loading}
        onSubmit={handleAction}
      />
    </ButtonGroup>
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
    <LoadingButton
      loading
      loadingPosition="start"
      loadingIndicator={label}
      // This icon can be anything
      startIcon={<ReplayIcon />}
    />
  );
};
