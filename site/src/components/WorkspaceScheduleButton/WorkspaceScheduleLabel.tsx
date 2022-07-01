import { makeStyles } from "@material-ui/core/styles"
import { Workspace } from "../../api/typesGenerated"
import { combineClasses } from "../../util/combineClasses"
import { autoStartDisplay, autoStopDisplay, isShuttingDown, Language } from "../../util/schedule"
import { isWorkspaceOn } from "../../util/workspace"

export const WorkspaceScheduleLabel: React.FC<{ workspace: Workspace }> = ({ workspace }) => {
  const styles = useStyles()

  if (isWorkspaceOn(workspace)) {
    const stopLabel = autoStopDisplay(workspace)
    const shouldDisplayStrongLabel = !isShuttingDown(workspace)

    // If it is shutting down, we don't need to display the auto stop label
    return (
      <span className={combineClasses([styles.labelText, "chromatic-ignore"])}>
        {shouldDisplayStrongLabel && (
          <strong className={styles.labelStrong}>{Language.autoStopLabel}</strong>
        )}
        {stopLabel}
      </span>
    )
  }

  return (
    <span className={combineClasses([styles.labelText, "chromatic-ignore"])}>
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
