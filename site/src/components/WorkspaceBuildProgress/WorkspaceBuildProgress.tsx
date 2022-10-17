import LinearProgress from "@material-ui/core/LinearProgress"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { Template, Workspace } from "api/typesGenerated"
import dayjs, { Dayjs } from "dayjs"
import { FC, useEffect, useState } from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"

import duration from "dayjs/plugin/duration"

dayjs.extend(duration)

const estimateFinish = (
  startedAt: Dayjs,
  templateAverage?: number,
): [number, string] => {
  if (templateAverage === undefined) {
    return [0, "Unknown"]
  }
  const realPercentage = dayjs().diff(startedAt) / templateAverage

  // Showing a full bar is frustrating.
  const maxPercentage = 0.99
  if (realPercentage > maxPercentage) {
    return [maxPercentage, "Any moment now..."]
  }

  return [
    realPercentage,
    `~${Math.ceil(
      dayjs.duration((1 - realPercentage) * templateAverage).asSeconds(),
    )} seconds remaining...`,
  ]
}

export interface WorkspaceBuildProgressProps {
  workspace: Workspace
  buildEstimate?: number
}

// EstimateTransitionTime gets the build estimate for the workspace,
// if it is in a transition state.
export const EstimateTransitionTime = (
  template: Template,
  workspace: Workspace,
): [number | undefined, boolean] => {
  switch (workspace.latest_build.status) {
    case "starting":
      return [template.build_time_stats.start_ms, true]
    case "stopping":
      return [template.build_time_stats.stop_ms, true]
    case "deleting":
      return [template.build_time_stats.delete_ms, true]
    default:
      // Not in a transition state
      return [undefined, false]
  }
}

export const WorkspaceBuildProgress: FC<WorkspaceBuildProgressProps> = ({
  workspace,
  buildEstimate,
}) => {
  const styles = useStyles()
  const job = workspace.latest_build.job
  const [progressValue, setProgressValue] = useState(0)

  // By default workspace is updated every second, which can cause visual stutter
  // when the build estimate is a few seconds. The timer ensures no observable
  // stutter in all cases.
  useEffect(() => {
    const updateProgress = () => {
      if (job.status !== "running") {
        setProgressValue(0)
        return
      }
      setProgressValue(
        estimateFinish(dayjs(job.started_at), buildEstimate)[0] * 100,
      )
    }
    setTimeout(updateProgress, 100)
  }, [progressValue, job, buildEstimate])

  // buildEstimate may be undefined if the template is new or coderd hasn't
  // finished initial metrics collection.
  if (buildEstimate === undefined) {
    return (
      <div className={styles.stack}>
        <LinearProgress value={0} variant="indeterminate" />
        <div className={styles.barHelpers}>
          <div className={styles.label}>{`Build ${job.status}`}</div>
          <div className={styles.label}>Unknown ETA</div>
        </div>
      </div>
    )
  }

  return (
    <div className={styles.stack}>
      <LinearProgress
        value={(job.status === "running" && progressValue) || 0}
        variant={job.status === "running" ? "determinate" : "indeterminate"}
      />
      <div className={styles.barHelpers}>
        <div className={styles.label}>{`Build ${job.status}`}</div>
        <div className={styles.label}>
          {job.status === "running" &&
            estimateFinish(dayjs(job.started_at), buildEstimate)[1]}
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
