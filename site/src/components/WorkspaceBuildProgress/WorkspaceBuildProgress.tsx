import LinearProgress from "@material-ui/core/LinearProgress"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { Template, Workspace } from "api/typesGenerated"
import dayjs, { Dayjs } from "dayjs"
import { FC } from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"

import duration from "dayjs/plugin/duration"

dayjs.extend(duration)

const estimateFinish = (
  startedAt: Dayjs,
  templateAverage: number,
): [number, string] => {
  // Buffer the template average to prevent the progress bar from waiting at end.
  // Over-promise, under-deliver.
  templateAverage *= 1.2

  const realPercentage = dayjs().diff(startedAt) / templateAverage

  // Showing a full bar is frustrating.
  if (realPercentage > 0.95) {
    return [0.95, "Any moment now..."]
  }

  return [
    realPercentage,
    `~${Math.ceil(
      dayjs.duration((1 - realPercentage) * templateAverage).asSeconds(),
    )} seconds remaining...`,
  ]
}

export const WorkspaceBuildProgress: FC<{
  workspace: Workspace
  template?: Template
}> = ({ workspace, template }) => {
  const styles = useStyles()

  // Template stats not loaded or non-existent
  if (!template || template.average_build_time_ms <= 0) {
    return <></>
  }

  const job = workspace.latest_build.job
  const status = job.status

  return (
    <div className={styles.stack}>
      <LinearProgress
        value={
          (status === "running" &&
            estimateFinish(
              dayjs(job.started_at),
              template.average_build_time_ms,
            )[0] * 100) ||
          0
        }
        variant={status === "running" ? "determinate" : "indeterminate"}
      />
      <div className={styles.barHelpers}>
        <div className={styles.label}>{`Build ${status}`}</div>
        <div className={styles.label}>
          {status === "running" &&
            estimateFinish(
              dayjs(job.started_at),
              template.average_build_time_ms,
            )[1]}
        </div>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  stack: {
    paddingLeft: theme.spacing(0.2),
    paddingRight: theme.spacing(0.2),
  },
  barHelpers: {
    display: "flex",
    justifyContent: "space-between",
  },
  label: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    fontSize: 12,
    textTransform: "uppercase",
    display: "block",
    fontWeight: 600,
    color: theme.palette.text.secondary,
  },
}))
