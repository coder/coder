import { makeStyles, Theme } from "@material-ui/core/styles"
import dayjs from "dayjs"
import { FC, Fragment } from "react"
import { ProvisionerJobLog } from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { Logs } from "../Logs/Logs"

const Language = {
  seconds: "seconds",
}

type Stage = ProvisionerJobLog["stage"]
type LogsGroupedByStage = Record<Stage, ProvisionerJobLog[]>
type GroupLogsByStageFn = (logs: ProvisionerJobLog[]) => LogsGroupedByStage

export const groupLogsByStage: GroupLogsByStageFn = (logs) => {
  const logsByStage: LogsGroupedByStage = {}

  for (const log of logs) {
    if (log.stage in logsByStage) {
      logsByStage[log.stage].push(log)
    } else {
      logsByStage[log.stage] = [log]
    }
  }

  return logsByStage
}

const getStageDurationInSeconds = (logs: ProvisionerJobLog[]) => {
  if (logs.length < 2) {
    return
  }

  const startedAt = dayjs(logs[0].created_at)
  const completedAt = dayjs(logs[logs.length - 1].created_at)
  return completedAt.diff(startedAt, "seconds")
}

export interface WorkspaceBuildLogsProps {
  logs: ProvisionerJobLog[]
  hideTimestamps?: boolean

  // If true, render different styles that fit the template editor pane
  // a bit better.
  templateEditorPane?: boolean
}

export const WorkspaceBuildLogs: FC<WorkspaceBuildLogsProps> = ({
  hideTimestamps,
  logs,
  templateEditorPane,
}) => {
  const groupedLogsByStage = groupLogsByStage(logs)
  const stages = Object.keys(groupedLogsByStage)
  const styles = useStyles({ templateEditorPane: Boolean(templateEditorPane) })

  return (
    <div className={styles.logs}>
      {stages.map((stage) => {
        const logs = groupedLogsByStage[stage]
        const isEmpty = logs.every((log) => log.output === "")
        const lines = logs.map((log) => ({
          time: log.created_at,
          output: log.output,
          level: log.log_level,
        }))
        const duration = getStageDurationInSeconds(logs)
        const shouldDisplayDuration = duration !== undefined

        return (
          <Fragment key={stage}>
            <div className={styles.header}>
              <div>{stage}</div>
              {shouldDisplayDuration && (
                <div className={styles.duration}>
                  {duration} {Language.seconds}
                </div>
              )}
            </div>
            {!isEmpty && <Logs hideTimestamps={hideTimestamps} lines={lines} />}
          </Fragment>
        )
      })}
    </div>
  )
}

const useStyles = makeStyles<
  Theme,
  {
    templateEditorPane: boolean
  }
>((theme) => ({
  logs: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: (props) =>
      props.templateEditorPane ? "0px" : theme.shape.borderRadius,
    fontFamily: MONOSPACE_FONT_FAMILY,
  },

  header: {
    fontSize: 14,
    padding: theme.spacing(2),
    paddingLeft: theme.spacing(3),
    paddingRight: theme.spacing(3),
    borderBottom: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
    display: "flex",
    alignItems: "center",
    fontFamily: "Inter",

    "&:first-child": {
      borderTopLeftRadius: theme.shape.borderRadius,
      borderTopRightRadius: theme.shape.borderRadius,
    },

    "&:last-child": {
      borderBottom: 0,
      borderTop: `1px solid ${theme.palette.divider}`,
      borderBottomLeftRadius: theme.shape.borderRadius,
      borderBottomRightRadius: theme.shape.borderRadius,
    },
  },

  duration: {
    marginLeft: "auto",
    color: theme.palette.text.secondary,
    fontSize: theme.typography.body2.fontSize,
  },
}))
