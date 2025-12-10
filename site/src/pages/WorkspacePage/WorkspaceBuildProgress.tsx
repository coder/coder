import LinearProgress from "@mui/material/LinearProgress";
import type { Template, TransitionStats, Workspace } from "api/typesGenerated";
import dayjs, { type Dayjs } from "dayjs";
import duration from "dayjs/plugin/duration";
import capitalize from "lodash/capitalize";
import { type FC, useEffect, useState } from "react";

dayjs.extend(duration);

// getActiveTransitionStats gets the build estimate for the workspace,
// if it is in a transition state.
export const getActiveTransitionStats = (
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
		return Number.isNaN(max) ? 0 : max;
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

interface WorkspaceBuildProgressProps {
	workspace: Workspace;
	transitionStats: TransitionStats;
	// variant changes how the progress bar is displayed: with the workspace
	// variant the workspace transition and time remaining are displayed under the
	// bar aligned to the left and right respectively.  With the task variant the
	// workspace transition is not displayed and the time remaining is displayed
	// centered above the bar, and the bar's border radius is removed.
	variant?: "workspace" | "task";
}

export const WorkspaceBuildProgress: FC<WorkspaceBuildProgressProps> = ({
	workspace,
	transitionStats,
	variant,
}) => {
	const job = workspace.latest_build.job;
	const [progressValue, setProgressValue] = useState<number | undefined>(0);
	const [progressText, setProgressText] = useState<string | undefined>(
		"Finding ETA...",
	);

	// By default workspace is updated every second, which can cause visual stutter
	// when the build estimate is a few seconds. The timer ensures no observable
	// stutter in all cases.
	// biome-ignore lint/correctness/useExhaustiveDependencies: consider refactoring
	useEffect(() => {
		const updateProgress = () => {
			if (
				job === undefined ||
				job.status !== "running" ||
				transitionStats.P50 === undefined ||
				transitionStats.P95 === undefined ||
				transitionStats.P50 === null ||
				transitionStats.P95 === null
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
		return null;
	}
	return (
		<div className="px-0.5">
			{variant === "task" && (
				<div className="mb-1 text-center">
					<div className={classNames.label} data-chromatic="ignore">
						{progressText}
					</div>
				</div>
			)}
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
				classes={{
					// If a transition is set, there is a moment on new load where the bar
					// accelerates to progressValue and then rapidly decelerates, which is
					// not indicative of true progress.
					bar: classNames.bar,
					// With the "task" variant, the progress bar is fullscreen, so remove
					// the border radius.
					root: variant === "task" ? classNames.root : undefined,
				}}
			/>
			{variant !== "task" && (
				<div className="flex mt-1 justify-between">
					<div className={classNames.label}>
						{capitalize(workspace.latest_build.status)} workspace...
					</div>
					<div className={classNames.label} data-chromatic="ignore">
						{progressText}
					</div>
				</div>
			)}
		</div>
	);
};

const classNames = {
	root: "rounded-none",
	bar: "transition-none",
	label: "text-xs block font-semibold text-content-secondary leading-loose",
};
