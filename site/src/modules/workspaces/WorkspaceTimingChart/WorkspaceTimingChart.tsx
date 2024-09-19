import type { Interpolation, Theme } from "@emotion/react";
import type { ProvisionerTiming } from "api/typesGenerated";
import { Bar } from "components/GanttChart/Bar";
import { Label } from "components/GanttChart/Label";
import { XGrid } from "components/GanttChart/XGrid";
import { XValues } from "components/GanttChart/XValues";
import type { FC } from "react";
import {
	consolidateTimings,
	intervals,
	startOffset,
	totalDuration,
} from "./timings";
import { TimingBlocks } from "./TimingBlocks";

const columnWidth = 130;
// Spacing between bars
const barsSpacing = 20;
const timesHeight = 40;
// Adds left padding to ensure the first bar does not touch the sidebar border,
// enhancing visual separation.
const barsXPadding = 4;
// Predicting the caption height is necessary to add appropriate spacing to the
// grouped bars, ensuring alignment with the sidebar labels.
const captionHeight = 20;
// The time interval used to calculate the x-axis values.
const timeInterval = 5;
// We control the stages to be displayed in the chart so we can set the correct
// colors and labels.
const stages = [
	{ name: "init" },
	{ name: "plan" },
	{ name: "graph" },
	{ name: "apply" },
];

type WorkspaceTimingChartProps = {
	provisionerTimings: readonly ProvisionerTiming[];
};

export const WorkspaceTimingChart: FC<WorkspaceTimingChartProps> = ({
	provisionerTimings,
}) => {
	const duration = totalDuration(provisionerTimings);

	const xValues = intervals(duration, timeInterval).map(formatSeconds);
	const provisionerTiming = consolidateTimings(provisionerTimings);

	const applyBarHeightToLabel = (bar: HTMLDivElement | null) => {
		if (!bar) {
			return;
		}
		const labelId = bar.getAttribute("aria-labelledby");
		if (!labelId) {
			return;
		}
		const label = document.querySelector<HTMLSpanElement>(`#${labelId}`);
		if (!label) {
			return;
		}
		label.style.height = `${bar.clientHeight}px`;
	};

	return (
		<div css={styles.chart}>
			<div css={styles.sidebar}>
				<section>
					<span css={styles.caption}>provisioning</span>
					<ul css={styles.labels}>
						{stages.map((s) => (
							<li key={s.name}>
								<Label id={`${s.name}-label`}>{s.name}</Label>
							</li>
						))}
					</ul>
				</section>
			</div>

			<div css={styles.main}>
				<XValues
					values={xValues}
					columnWidth={columnWidth}
					css={styles.xValues}
				/>
				<div css={styles.bars}>
					{stages.map((s) => {
						const timings = provisionerTimings.filter(
							(t) => t.stage === s.name,
						);
						const stageTiming = consolidateTimings(timings);
						const stageDuration = totalDuration(timings);
						const offset = startOffset(provisionerTiming, stageTiming);
						const stageSize = size(stageDuration);

						return (
							<Bar
								key={s.name}
								x={size(offset)}
								width={stageSize}
								afterLabel={
									<Label color="secondary">{stageDuration.toFixed(2)}s</Label>
								}
								aria-labelledby={`${s.name}-label`}
								ref={applyBarHeightToLabel}
							>
								{timings.length > 1 && (
									<TimingBlocks
										timings={timings}
										stageSize={stageSize}
										blockSize={4}
									/>
								)}
							</Bar>
						);
					})}

					<XGrid
						columnWidth={columnWidth}
						columns={xValues.length}
						css={{
							position: "absolute",
							top: 0,
							left: 0,
							zIndex: -1,
						}}
					/>
				</div>
			</div>
		</div>
	);
};

const formatSeconds = (seconds: number): string => {
	const hours = Math.floor(seconds / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	const remainingSeconds = seconds % 60;

	return `${hours.toString().padStart(2, "0")}:${minutes
		.toString()
		.padStart(2, "0")}:${remainingSeconds.toString().padStart(2, "0")}`;
};

/**
 * Returns the size in pixels based on the time interval and the column width
 * for the interval.
 */
const size = (duration: number): number => {
	return (duration / timeInterval) * columnWidth;
};

const styles = {
	chart: {
		display: "flex",
		alignItems: "stretch",
		height: "100%",
	},
	sidebar: {
		width: columnWidth,
		flexShrink: 0,
		padding: `${timesHeight}px 16px`,
	},
	caption: (theme) => ({
		height: captionHeight,
		display: "flex",
		alignItems: "center",
		fontSize: 10,
		fontWeight: 500,
		color: theme.palette.text.secondary,
	}),
	labels: {
		margin: 0,
		padding: 0,
		listStyle: "none",
		display: "flex",
		flexDirection: "column",
		gap: barsSpacing,
		textAlign: "right",
	},
	main: (theme) => ({
		display: "flex",
		flexDirection: "column",
		flex: 1,
		borderLeft: `1px solid ${theme.palette.divider}`,
	}),
	xValues: (theme) => ({
		borderBottom: `1px solid ${theme.palette.divider}`,
		height: timesHeight,
		padding: `0px ${barsXPadding}px`,
		minWidth: "100%",
		flexShrink: 0,
		position: "sticky",
		top: 0,
		zIndex: 1,
		backgroundColor: theme.palette.background.default,
	}),
	bars: {
		display: "flex",
		flexDirection: "column",
		position: "relative",
		gap: barsSpacing,
		padding: `${captionHeight}px ${barsXPadding}px`,
		flex: 1,
	},
} satisfies Record<string, Interpolation<Theme>>;
