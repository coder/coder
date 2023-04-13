import Button from "@material-ui/core/Button"
import BlockIcon from "@material-ui/icons/Block"
import CloudQueueIcon from "@material-ui/icons/CloudQueue"
import CropSquareIcon from "@material-ui/icons/CropSquare"
import PlayCircleOutlineIcon from "@material-ui/icons/PlayCircleOutline"
import { LoadingButton } from "components/LoadingButton/LoadingButton"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { makeStyles } from "@material-ui/core/styles"

interface WorkspaceAction {
  handleAction: () => void
}

export const UpdateButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const { t } = useTranslation("workspacePage")
  const styles = useStyles()

  return (
    <Button
      variant="outlined"
      startIcon={<CloudQueueIcon />}
      onClick={handleAction}
      className={styles.fixedWidth}
    >
      {t("actionButton.update")}
    </Button>
  )
}

export const StartButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const { t } = useTranslation("workspacePage")
  const styles = useStyles()

  return (
    <Button
      variant="outlined"
      startIcon={<PlayCircleOutlineIcon />}
      onClick={handleAction}
      className={styles.fixedWidth}
    >
      {t("actionButton.start")}
    </Button>
  )
}

export const StopButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({
  handleAction,
}) => {
  const { t } = useTranslation("workspacePage")
  const styles = useStyles()

  return (
    <Button
      variant="outlined"
      startIcon={<CropSquareIcon />}
      onClick={handleAction}
      className={styles.fixedWidth}
    >
      {t("actionButton.stop")}
    </Button>
  )
}

export const CancelButton: FC<React.PropsWithChildren<WorkspaceAction>> = ({
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

export const DisabledButton: FC<React.PropsWithChildren<DisabledProps>> = ({
  label,
}) => {
  return (
    <Button variant="outlined" disabled>
      {label}
    </Button>
  )
}

interface LoadingProps {
  label: string
}

export const ActionLoadingButton: FC<React.PropsWithChildren<LoadingProps>> = ({
  label,
}) => {
  const styles = useStyles()
  return (
    <LoadingButton
      loading
      variant="outlined"
      loadingLabel={label}
      className={styles.fixedWidth}
    />
  )
}

const useStyles = makeStyles((theme) => ({
  fixedWidth: {
    // Make it fixed so the loading changes will not "flick" the UI
    width: theme.spacing(16),
  },
}))
