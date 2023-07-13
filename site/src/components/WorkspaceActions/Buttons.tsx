import Button from "@mui/material/Button"
import BlockIcon from "@mui/icons-material/Block"
import CloudQueueIcon from "@mui/icons-material/CloudQueue"
import CropSquareIcon from "@mui/icons-material/CropSquare"
import PlayCircleOutlineIcon from "@mui/icons-material/PlayCircleOutline"
import ReplayIcon from "@mui/icons-material/Replay"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { FC, useRef, useState } from "react"
import BlockOutlined from "@mui/icons-material/BlockOutlined"
import ButtonGroup from "@mui/material/ButtonGroup"
import ExpandMoreOutlined from "@mui/icons-material/ExpandMoreOutlined"
import Popover from "@mui/material/Popover"
import {
  HelpTooltipText,
  HelpTooltipTitle,
} from "components/Tooltips/HelpTooltip/HelpTooltip"
import Box from "@mui/material/Box"
import { useQuery } from "@tanstack/react-query"
import { Workspace } from "api/typesGenerated"
import { getWorkspaceParameters } from "api/api"
import { BuildParametersForm } from "./BuildParametersPopover"
import { Loader } from "components/Loader/Loader"

interface WorkspaceAction {
  loading?: boolean
  handleAction: () => void
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
  )
}

export const StartButton: FC<WorkspaceAction & { workspace: Workspace }> = ({
  handleAction,
  workspace,
  loading,
}) => {
  const anchorRef = useRef<HTMLButtonElement>(null)
  const [isOpen, setIsOpen] = useState(false)
  const { data: parameters } = useQuery({
    queryKey: ["workspace", workspace.id, "parameters"],
    queryFn: () => getWorkspaceParameters(workspace),
    enabled: isOpen,
  })
  const ephemeralParameters = parameters
    ? parameters.templateVersionRichParameters.filter((p) => p.ephemeral)
    : undefined

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
        onClick={handleAction}
      >
        Start
      </LoadingButton>
      <Button
        disabled={loading}
        color="neutral"
        sx={{ px: 0 }}
        ref={anchorRef}
        onClick={() => {
          setIsOpen(true)
        }}
      >
        <ExpandMoreOutlined sx={{ fontSize: 16 }} />
      </Button>
      <Popover
        open={isOpen}
        anchorEl={anchorRef.current}
        onClose={() => {
          setIsOpen(false)
        }}
        anchorOrigin={{
          vertical: "bottom",
          horizontal: "right",
        }}
        transformOrigin={{
          vertical: "top",
          horizontal: "right",
        }}
        sx={{
          ".MuiPaper-root": {
            p: 2.5,
            width: (theme) => theme.spacing(38),
            marginTop: 1,
          },
        }}
      >
        <Box sx={{ color: (theme) => theme.palette.text.secondary }}>
          <HelpTooltipTitle>Build Options</HelpTooltipTitle>
          <HelpTooltipText>
            These parameters only apply for a single workspace start.
          </HelpTooltipText>
        </Box>
        <Box>
          {parameters && parameters.buildParameters && ephemeralParameters ? (
            <BuildParametersForm
              buildParameters={parameters.buildParameters}
              ephemeralParameters={ephemeralParameters}
            />
          ) : (
            <Loader />
          )}
        </Box>
      </Popover>
    </ButtonGroup>
  )
}

export const StopButton: FC<WorkspaceAction> = ({ handleAction, loading }) => {
  return (
    <LoadingButton
      loading={loading}
      loadingIndicator="Stopping..."
      loadingPosition="start"
      startIcon={<CropSquareIcon />}
      onClick={handleAction}
    >
      Stop
    </LoadingButton>
  )
}

export const RestartButton: FC<WorkspaceAction> = ({
  handleAction,
  loading,
}) => {
  return (
    <LoadingButton
      loading={loading}
      loadingIndicator="Restarting..."
      loadingPosition="start"
      startIcon={<ReplayIcon />}
      onClick={handleAction}
      data-testid="workspace-restart-button"
    >
      Restart
    </LoadingButton>
  )
}

export const CancelButton: FC<WorkspaceAction> = ({ handleAction }) => {
  return (
    <Button startIcon={<BlockIcon />} onClick={handleAction}>
      Cancel
    </Button>
  )
}

interface DisabledProps {
  label: string
}

export const DisabledButton: FC<DisabledProps> = ({ label }) => {
  return (
    <Button startIcon={<BlockOutlined />} disabled>
      {label}
    </Button>
  )
}

interface LoadingProps {
  label: string
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
  )
}
