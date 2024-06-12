import { css } from "@emotion/css";
import type { Interpolation, Theme } from "@emotion/react";
import LinearProgress from "@mui/material/LinearProgress";
import dayjs, { type Dayjs } from "dayjs";
import duration from "dayjs/plugin/duration";
import capitalize from "lodash/capitalize";
import { type FC, useEffect, useState } from "react";
import type { TransitionStats, Template, Workspace } from "api/typesGenerated";

dayjs.extend(duration);

// ActiveTransition gets the build estimate for the workspace,
// if it is in a transition state.
export const ActiveTransition = (
  template: Template,
  workspace: Workspace,
): TransitionStats | undefined => {
  const status = workspace.latest_build.status;

  switch (status) {
    case "starting":
      return template.build_time_stats.start;
    case "stopping":
      return template.build_time_stats.stop;
    case "deleting":
      return template.build_time_stats.delete;
    default:
      return undefined;
  }
};

const estimateFinish = (
  startedAt: Dayjs,
  p50: number,
  p95: number,
): [number | undefined, string] => {
  const sinceStart = dayjs().diff(startedAt);
  const secondsLeft = (est: number) => {
    const max = Math.max(
      Math.ceil(dayjs.duration((1 - sinceStart / est) * est).asSeconds()),
      0,
    );
    return isNaN(max) ? 0 : max;
  };

  // Under-promise, over-deliver with the 95th percentile estimate.
  const highGuess = secondsLeft(p95);

  const anyMomentNow: [number | undefined, string] = [
    undefined,
    "Any moment now...",
  ];

  const p50percent = (sinceStart * 100) / p50;
  if (highGuess <= 0) {
    return anyMomentNow;
  }

  return [p50percent, `Up to ${highGuess} seconds remaining...`];
};

export interface WorkspaceBuildProgressProps {
  workspace: Workspace;
  transitionStats: TransitionStats;
}

export const WorkspaceBuildProgress: FC<WorkspaceBuildProgressProps> = ({
  workspace,
  transitionStats: transitionStats,
}) => {
  const job = workspace.latest_build.job;
  const [progressValue, setProgressValue] = useState<number | undefined>(0);
  const [progressText, setProgressText] = useState<string | undefined>(
    "Finding ETA...",
  );

  // By default workspace is updated every second, which can cause visual stutter
  // when the build estimate is a few seconds. The timer ensures no observable
  // stutter in all cases.
  useEffect(() => {
    const updateProgress = () => {
      if (
        job.status !== "running" ||
        transitionStats.P50 === undefined ||
        transitionStats.P95 === undefined
      ) {
        setProgressValue(undefined);
        setProgressText(undefined);
        return;
      }

      const [est, text] = estimateFinish(
        dayjs(job.started_at),
        transitionStats.P50,
        transitionStats.P95,
      );
      setProgressValue(est);
      setProgressText(text);
    };
    const updateTimer = requestAnimationFrame(updateProgress);
    return () => {
      cancelAnimationFrame(updateTimer);
    };
  }, [progressValue, job, transitionStats]);

  // HACK: the codersdk type generator doesn't support null values, but this
  // can be null when the template is new.
  if ((transitionStats.P50 as number | null) === null) {
    return <></>;
  }
  return (
    <div css={styles.stack}>
      <LinearProgress
        data-chromatic="ignore"
        value={progressValue !== undefined ? progressValue : 0}
        variant={
          // There is an initial state where progressValue may be undefined
          // (e.g. the build isn't yet running). If we flicker from the
          // indeterminate bar to the determinate bar, the vigilant user
          // perceives the bar jumping from 100% to 0%.
          progressValue !== undefined && progressValue < 100
            ? "determinate"
            : "indeterminate"
        }
        // If a transition is set, there is a moment on new load where the
        // bar accelerates to progressValue and then rapidly decelerates, which
        // is not indicative of true progress.
        classes={{ bar: classNames.bar }}
      />
      <div css={styles.barHelpers}>
        <div css={styles.label}>
          {capitalize(workspace.latest_build.status)} workspace...
        </div>
        <div css={styles.label} data-chromatic="ignore">
          {progressText}
        </div>
      </div>
    </div>
  );
};

const classNames = {
  bar: css`
    transition: none;
  `,
};

const styles = {
  stack: {
    paddingLeft: 2,
    paddingRight: 2,
  },
  barHelpers: {
    display: "flex",
    justifyContent: "space-between",
    marginTop: 4,
  },
  label: (theme) => ({
    fontSize: 12,
    display: "block",
    fontWeight: 600,
    color: theme.palette.text.secondary,
  }),
} satisfies Record<string, Interpolation<Theme>>;
