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
        {shouldDisplayStrongLabel && <strong>{Language.autoStopLabel}</strong>}{" "}
        <span className={styles.value}>{stopLabel}</span>
      </span>
    )
  }

  return (
    <span className={combineClasses([styles.labelText, "chromatic-ignore"])}>
      <strong>{Language.autoStartLabel}</strong>{" "}
      <span className={styles.value}>{autoStartDisplay(workspace.autostart_schedule)}</span>
    </span>
  )
}

const useStyles = makeStyles((theme) => ({
  labelText: {
    marginRight: theme.spacing(2),
    lineHeight: "160%",

    [theme.breakpoints.down("sm")]: {
      marginRight: 0,
      width: "100%",
    },
  },

  value: {
    [theme.breakpoints.down("sm")]: {
      whiteSpace: "nowrap",
    },
  },
}))
