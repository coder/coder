import LinearProgress from "@material-ui/core/LinearProgress"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { Template, Workspace } from "api/typesGenerated"
import dayjs, { Dayjs } from "dayjs"
import { FC } from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"

import duration from "dayjs/plugin/duration"
import { Running } from "components/WorkspaceStatusBadge/WorkspaceStatusBadge.stories"

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
  if (!template) {
    return <></>
  }

  let buildEstimate: number | undefined = 0
  switch (workspace.latest_build.status) {
    case "starting":
      buildEstimate = template.build_time_stats.start_ms
      break
    case "stopping":
      buildEstimate = template.build_time_stats.stop_ms
      break
    case "deleting":
      buildEstimate = template.build_time_stats.delete_ms
      break
    default:
      // Not in a transition state
      return <></>
  }

  const job = workspace.latest_build.job
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
        value={
          (job.status === "running" &&
            estimateFinish(dayjs(job.started_at), buildEstimate)[0] * 100) ||
          0
        }
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
