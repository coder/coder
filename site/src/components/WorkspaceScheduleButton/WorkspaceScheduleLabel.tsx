import { makeStyles } from "@material-ui/core/styles"
import { Workspace } from "../../api/typesGenerated"
import { autoStartDisplay, autoStopDisplay, Language } from "../../util/schedule"
import { isWorkspaceOn } from "../../util/workspace"

export const WorkspaceScheduleLabel: React.FC<{ workspace: Workspace }> = ({ workspace }) => {
  const styles = useStyles()

  if (isWorkspaceOn(workspace)) {
    const stopLabel = autoStopDisplay(workspace)
    const isShuttingDown = stopLabel === Language.workspaceShuttingDownLabel

    // If it is shutting down, we don't need to display the auto stop label
    return (
      <span className={styles.labelText}>
        {!isShuttingDown && (
          <strong className={styles.labelStrong}>{Language.autoStopLabel}</strong>
        )}
        {stopLabel}
      </span>
    )
  }

  return (
    <span className={styles.labelText}>
      <strong className={styles.labelStrong}>{Language.autoStartLabel}</strong>
      {autoStartDisplay(workspace.autostart_schedule)}
    </span>
  )
}

const useStyles = makeStyles((theme) => ({
  labelText: {
    marginRight: theme.spacing(2),
  },

  labelStrong: {
    marginRight: theme.spacing(0.5),
  },
}))
