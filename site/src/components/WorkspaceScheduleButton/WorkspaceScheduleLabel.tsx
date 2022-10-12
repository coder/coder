import { makeStyles } from "@material-ui/core/styles"
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne"
import { Maybe } from "components/Conditionals/Maybe"
import { useTranslation } from "react-i18next"
import { Workspace } from "../../api/typesGenerated"
import { combineClasses } from "../../util/combineClasses"
import {
  autoStartDisplay,
  autoStopDisplay,
  isShuttingDown,
} from "../../util/schedule"
import { isWorkspaceOn } from "../../util/workspace"

export const WorkspaceScheduleLabel: React.FC<{ workspace: Workspace }> = ({
  workspace,
}) => {
  const styles = useStyles()
  const { t } = useTranslation("common")

  return (
    <ChooseOne>
      <Cond condition={isWorkspaceOn(workspace)}>
        <span
          className={combineClasses([styles.labelText, "chromatic-ignore"])}
        >
          <Maybe condition={!isShuttingDown(workspace)}>
            <strong>{t("schedule.autoStopLabel")}</strong>
          </Maybe>{" "}
          <span className={styles.value}>{autoStopDisplay(workspace)}</span>
        </span>
      </Cond>
      <Cond>
        <span
          className={combineClasses([styles.labelText, "chromatic-ignore"])}
        >
          <strong>{t("schedule.autoStartLabel")}</strong>{" "}
          <span className={styles.value}>
            {autoStartDisplay(workspace.autostart_schedule)}
          </span>
        </span>
      </Cond>
    </ChooseOne>
  )
}

const useStyles = makeStyles((theme) => ({
  labelText: {
    marginRight: theme.spacing(1),
    marginLeft: theme.spacing(1),
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
