import LinearProgress from "@material-ui/core/LinearProgress"
import makeStyles from "@material-ui/core/styles/makeStyles"
import { TransitionStats, Template, Workspace } from "api/typesGenerated"
import dayjs, { Dayjs } from "dayjs"
import { FC, useEffect, useState } from "react"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"

import duration from "dayjs/plugin/duration"

dayjs.extend(duration)

// ActiveTransition gets the build estimate for the workspace,
// if it is in a transition state.
export const ActiveTransition = (
  template: Template,
  workspace: Workspace,
): TransitionStats | undefined => {
  const status = workspace.latest_build.status

  switch (status) {
    case "starting":
      return template.build_time_stats.start
    case "stopping":
      return template.build_time_stats.stop
    case "deleting":
      return template.build_time_stats.delete
    default:
      return undefined
  }
}

const estimateFinish = (
  startedAt: Dayjs,
  median: number,
  stddev: number,
): [number | undefined, string] => {
  const sinceStart = dayjs().diff(startedAt)
  const secondsLeft = (est: number) =>
    Math.max(
      Math.ceil(dayjs.duration((1 - sinceStart / est) * est).asSeconds()),
      0,
    )

  const lowGuess = secondsLeft(median)
  const highGuess = secondsLeft(median + stddev)

  const anyMomentNow: [number | undefined, string] = [
    undefined,
    "Any moment now...",
  ]

  // If variation is too high (and greater than second), don't show
  // progress bar and give range.
  const highVariation = stddev / median > 0.1 && highGuess - lowGuess > 1
  if (highVariation) {
    if (highGuess <= 0) {
      return anyMomentNow
    }
    return [undefined, `${lowGuess} to ${highGuess} seconds remaining...`]
  } else {
    const realPercentage = sinceStart / median
    if (realPercentage > 1) {
      return anyMomentNow
    }
    return [realPercentage * 100, `${highGuess} seconds remaining...`]
  }
}

export interface WorkspaceBuildProgressProps {
  workspace: Workspace
  transitionStats: TransitionStats
}

export const WorkspaceBuildProgress: FC<WorkspaceBuildProgressProps> = ({
  workspace,
  transitionStats: transitionStats,
}) => {
  const styles = useStyles()
  const job = workspace.latest_build.job
  const [progressValue, setProgressValue] = useState<number | undefined>(0)
  const [progressText, setProgressText] = useState<string | undefined>(
    "Finding ETA...",
  )

  // By default workspace is updated every second, which can cause visual stutter
  // when the build estimate is a few seconds. The timer ensures no observable
  // stutter in all cases.
  useEffect(() => {
    const updateProgress = () => {
      if (
        job.status !== "running" ||
        transitionStats.Median === undefined ||
        transitionStats.Stddev === undefined
      ) {
        setProgressValue(undefined)
        setProgressText(undefined)
        return
      }

      const [est, text] = estimateFinish(
        dayjs(job.started_at),
        transitionStats.Median,
        transitionStats.Stddev,
      )
      setProgressValue(est)
      setProgressText(text)
    }
    setTimeout(updateProgress, 5)
  }, [progressValue, job, transitionStats])

  return (
    <div className={styles.stack}>
      <LinearProgress
        value={progressValue !== undefined ? progressValue : 0}
        variant={
          // There is an initial state where progressValue may be undefined
          // (e.g. the build isn't yet running). If we flicker from the
          // indeterminate bar to the determinate bar, the vigilant user
          // perceives the bar jumping from 100% to 0%.
          progressValue !== undefined &&
          progressValue < 100 &&
          transitionStats !== undefined
            ? "determinate"
            : "indeterminate"
        }
        // If a transition is set, there is a moment on new load where the
        // bar accelerates to progressValue and then rapidly decelerates, which
        // is not indicative of true progress.
        classes={{ bar: styles.noTransition }}
      />
      <div className={styles.barHelpers}>
        <div className={styles.label}>{`Build ${job.status}`}</div>
        <div className={styles.label}>{progressText}</div>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  stack: {
    paddingLeft: theme.spacing(0.2),
    paddingRight: theme.spacing(0.2),
  },
  noTransition: {
    transition: "none",
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
