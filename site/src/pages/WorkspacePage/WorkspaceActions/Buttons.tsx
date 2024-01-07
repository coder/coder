import Tooltip from "@mui/material/Tooltip";
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
import { type FC } from "react";
import type { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { BuildParametersPopover } from "./BuildParametersPopover";
import { TopbarButton } from "components/FullPageLayout/Topbar";

interface ActionButtonProps {
  loading?: boolean;
  handleAction: (buildParameters?: WorkspaceBuildParameter[]) => void;
  disabled?: boolean;
  tooltipText?: string;
}

export const UpdateButton: FC<ActionButtonProps> = ({
  handleAction,
  loading,
}) => {
  return (
    <TopbarButton
      disabled={loading}
      data-testid="workspace-update-button"
      startIcon={<CloudQueueIcon />}
      onClick={() => handleAction()}
    >
      {loading ? <>Updating&hellip;</> : <>Update&hellip;</>}
    </TopbarButton>
  );
};

export const ActivateButton: FC<ActionButtonProps> = ({
  handleAction,
  loading,
}) => {
  return (
    <TopbarButton
      disabled={loading}
      startIcon={<PowerSettingsNewIcon />}
      onClick={() => handleAction()}
    >
      {loading ? <>Activating&hellip;</> : "Activate"}
    </TopbarButton>
  );
};

interface ActionButtonPropsWithWorkspace extends ActionButtonProps {
  workspace: Workspace;
}

export const StartButton: FC<ActionButtonPropsWithWorkspace> = ({
  handleAction,
  workspace,
  loading,
  disabled,
  tooltipText,
}) => {
  const buttonContent = (
    <ButtonGroup
      variant="outlined"
      sx={{
        // Workaround to make the border transitions smoothly on button groups
        "& > button:hover + button": {
          borderLeft: "1px solid #FFF",
        },
      }}
      disabled={disabled}
    >
      <TopbarButton
        startIcon={<PlayCircleOutlineIcon />}
        onClick={() => handleAction()}
        disabled={disabled || loading}
      >
        {loading ? <>Starting&hellip;</> : "Start"}
      </TopbarButton>
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

export const StopButton: FC<ActionButtonProps> = ({
  handleAction,
  loading,
}) => {
  return (
    <TopbarButton
      disabled={loading}
      startIcon={<CropSquareIcon />}
      onClick={() => handleAction()}
      data-testid="workspace-stop-button"
    >
      {loading ? <>Stopping&hellip;</> : "Stop"}
    </TopbarButton>
  );
};

export const RestartButton: FC<ActionButtonPropsWithWorkspace> = ({
  handleAction,
  loading,
  workspace,
  disabled,
  tooltipText,
}) => {
  const buttonContent = (
    <ButtonGroup
      variant="outlined"
      css={{
        // Workaround to make the border transitions smoothly on button groups
        "& > button:hover + button": {
          borderLeft: "1px solid #FFF",
        },
      }}
      disabled={disabled}
    >
      <TopbarButton
        startIcon={<ReplayIcon />}
        onClick={() => handleAction()}
        data-testid="workspace-restart-button"
        disabled={disabled || loading}
      >
        {loading ? <>Restarting&hellip;</> : <>Restart&hellip;</>}
      </TopbarButton>
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

export const CancelButton: FC<ActionButtonProps> = ({ handleAction }) => {
  return (
    <TopbarButton startIcon={<BlockIcon />} onClick={() => handleAction()}>
      Cancel
    </TopbarButton>
  );
};

interface DisabledButtonProps {
  label: string;
}

export const DisabledButton: FC<DisabledButtonProps> = ({ label }) => {
  return (
    <TopbarButton startIcon={<OutlinedBlockIcon />} disabled>
      {label}
    </TopbarButton>
  );
};

type RetryButtonProps = Omit<ActionButtonProps, "loading"> & {
  debug?: boolean;
};

export const RetryButton: FC<RetryButtonProps> = ({
  handleAction,
  debug = false,
}) => {
  return (
    <TopbarButton
      startIcon={debug ? <RetryDebugIcon /> : <RetryIcon />}
      onClick={() => handleAction()}
    >
      Retry{debug && " (Debug)"}
    </TopbarButton>
  );
};
