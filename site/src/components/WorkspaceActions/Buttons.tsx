import Button from "@mui/material/Button"
import BlockIcon from "@mui/icons-material/Block"
import CloudQueueIcon from "@mui/icons-material/CloudQueue"
import CropSquareIcon from "@mui/icons-material/CropSquare"
import PlayCircleOutlineIcon from "@mui/icons-material/PlayCircleOutline"
import ReplayIcon from "@mui/icons-material/Replay"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { FC, PropsWithChildren } from "react"
import { useTranslation } from "react-i18next"

interface WorkspaceAction {
  handleAction: () => void
}

export const UpdateButton: FC<PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const { t } = useTranslation("workspacePage")

  return (
    <Button
      size="small"
      data-testid="workspace-update-button"
      variant="outlined"
      startIcon={<CloudQueueIcon />}
      onClick={handleAction}
    >
      {t("actionButton.update")}
    </Button>
  )
}

export const StartButton: FC<PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const { t } = useTranslation("workspacePage")

  return (
    <Button
      variant="outlined"
      startIcon={<PlayCircleOutlineIcon />}
      onClick={handleAction}
    >
      {t("actionButton.start")}
    </Button>
  )
}

export const StopButton: FC<PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const { t } = useTranslation("workspacePage")

  return (
    <Button
      size="small"
      variant="outlined"
      startIcon={<CropSquareIcon />}
      onClick={handleAction}
    >
      {t("actionButton.stop")}
    </Button>
  )
}

export const RestartButton: FC<PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const { t } = useTranslation("workspacePage")

  return (
    <Button
      size="small"
      variant="outlined"
      startIcon={<ReplayIcon />}
      onClick={handleAction}
    >
      {t("actionButton.restart")}
    </Button>
  )
}

export const CancelButton: FC<PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  return (
    <Button variant="outlined" startIcon={<BlockIcon />} onClick={handleAction}>
      Cancel
    </Button>
  )
}

interface DisabledProps {
  label: string
}

export const DisabledButton: FC<PropsWithChildren<DisabledProps>> = ({
  label,
}) => {
  return (
    <Button variant="outlined" size="small" disabled>
      {label}
    </Button>
  )
}

interface LoadingProps {
  label: string
}

export const ActionLoadingButton: FC<PropsWithChildren<LoadingProps>> = ({
  label,
}) => {
  return (
    <LoadingButton
      loading
      size="small"
      variant="outlined"
      loadingLabel={label}
    />
  )
}
