import { makeStyles } from "@material-ui/core/styles"
import dayjs from "dayjs"
import { FC } from "react"
import { ProvisionerJobLog } from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"
import { Logs } from "../Logs/Logs"

const Language = {
  seconds: "seconds",
}

type Stage = ProvisionerJobLog["stage"]

const groupLogsByStage = (logs: ProvisionerJobLog[]) => {
  const logsByStage: Record<Stage, ProvisionerJobLog[]> = {}

  for (const log of logs) {
    // If there is no log in the stage record, add an empty array
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition
    if (logsByStage[log.stage] === undefined) {
      logsByStage[log.stage] = []
    }

    logsByStage[log.stage].push(log)
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
}

export const WorkspaceBuildLogs: FC<WorkspaceBuildLogsProps> = ({ logs }) => {
  const groupedLogsByStage = groupLogsByStage(logs)
  const stages = Object.keys(groupedLogsByStage)
  const styles = useStyles()

  return (
    <div className={styles.logs}>
      {stages.map((stage) => {
        const logs = groupedLogsByStage[stage]
        const isEmpty = logs.every((log) => log.output === "")
        const lines = logs.map((log) => ({
          time: log.created_at,
          output: log.output,
        }))
        const duration = getStageDurationInSeconds(logs)
        const shouldDisplayDuration = duration !== undefined

        return (
          <div key={stage}>
            <div className={styles.header}>
              <div>{stage}</div>
              {shouldDisplayDuration && (
                <div className={styles.duration}>
                  {duration} {Language.seconds}
                </div>
              )}
            </div>
            {!isEmpty && <Logs lines={lines} className={styles.codeBlock} />}
          </div>
        )
      })}
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  logs: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 2,
    fontFamily: MONOSPACE_FONT_FAMILY,
  },

  header: {
    fontSize: theme.typography.body1.fontSize,
    padding: theme.spacing(2),
    paddingLeft: theme.spacing(4),
    paddingRight: theme.spacing(4),
    borderBottom: `1px solid ${theme.palette.divider}`,
    backgroundColor: theme.palette.background.paper,
    display: "flex",
    alignItems: "center",
  },

  duration: {
    marginLeft: "auto",
    color: theme.palette.text.secondary,
    fontSize: theme.typography.body2.fontSize,
  },

  codeBlock: {
    padding: theme.spacing(2),
    paddingLeft: theme.spacing(4),
  },
}))
