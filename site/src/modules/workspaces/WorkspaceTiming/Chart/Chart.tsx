import type { Interpolation, Theme } from "@emotion/react";
import { XGrid } from "./XGrid";
import { XAxis } from "./XAxis";
import type { FC } from "react";
import { TimingBlocks } from "./TimingBlocks";
import {
	YAxis,
	YAxisCaption,
	YAxisCaptionHeight,
	YAxisLabel,
	YAxisLabels,
	YAxisSection,
} from "./YAxis";
import {
	barsSpacing,
	columnWidth,
	contentSidePadding,
	XAxisHeight,
} from "./constants";
import { Bar, type BarColor } from "./Bar";

export type Timing = Duration & {
	/**
	 * Label that will be displayed on the Y axis.
	 */
	label: string;
	/**
	 * A timing can represent either a single time block or a group of time
	 * blocks. When it represents a group, we display blocks within the bars to
	 * clearly indicate to the user that the timing encompasses multiple time
	 * blocks.
	 */
	childrenCount: number;
	/**
	 * Timings should always be included in duration and timeline calculations.
	 * However, some timings, such as those for Coder resources, may not be
	 * valuable or can be too spammy to present to the user. Therefore, we hide
	 * these specific timings from the visualization while still including them in
	 * the calculations.
	 */
	visible?: boolean;
	color?: BarColor;
};

// Extracts the 'startedAt' and 'endedAt' date fields from the main Timing type.
// This is useful for performing chart operations without needing additional
// information like labels or children count, which are only used for display
// purposes.
export type Duration = {
	startedAt: Date;
	endedAt: Date;
};

// Data can be divided into sections. For example, display the provisioning
// timings in one section and the scripting timings in another.
type DataSection = {
	name: string;
	timings: Timing[];
};

export type ChartProps = {
	data: DataSection[];
	onBarClick: (label: string, section: string) => void;
};

export const Chart: FC<ChartProps> = ({ data, onBarClick }) => {
	const totalDuration = duration(data.flatMap((d) => d.timings));
	const totalTime = durationTime(totalDuration);

	// XAxis ticks
	const tickSpacing = calcTickSpacing(totalTime);
	const ticksCount = Math.ceil(totalTime / tickSpacing);
	const ticks = Array.from(
		{ length: ticksCount },
		(_, i) => i * tickSpacing + tickSpacing,
	);

	// Helper function to convert the tick spacing into pixel size. This is used
	// for setting the bar width and offset.
	const calcSize = (time: number): number => {
		return (columnWidth * time) / tickSpacing;
	};

	const formatTime = (time: number): string => {
		if (tickSpacing <= 1_000) {
			return `${time.toLocaleString()}ms`;
		}
		return `${(time / 1_000).toLocaleString(undefined, {
			maximumFractionDigits: 2,
		})}s`;
	};

	return (
		<div css={styles.chart}>
			<YAxis>
				{data.map((section) => {
					return (
						<YAxisSection key={section.name}>
							<YAxisCaption>{section.name}</YAxisCaption>
							<YAxisLabels>
								{section.timings
									.filter((t) => t.visible)
									.map((t) => (
										<YAxisLabel
											key={t.label}
											id={`${encodeURIComponent(t.label)}-label`}
										>
											{t.label}
										</YAxisLabel>
									))}
							</YAxisLabels>
						</YAxisSection>
					);
				})}
			</YAxis>

			<div css={styles.main}>
				<XAxis labels={ticks.map(formatTime)} />
				<div css={styles.content}>
					{data.map((section) => {
						return (
							<div key={section.name} css={styles.bars}>
								{section.timings
									.filter((t) => t.visible)
									.map((t) => {
										const offset =
											t.startedAt.getTime() - totalDuration.startedAt.getTime();
										const size = calcSize(durationTime(t));
										return (
											<Bar
												color={t.color}
												key={t.label}
												x={calcSize(offset)}
												width={size}
												afterLabel={formatTime(durationTime(t))}
												aria-labelledby={`${t.label}-label`}
												ref={applyBarHeightToLabel}
												disabled={t.childrenCount <= 1}
												onClick={() => {
													if (t.childrenCount <= 1) {
														return;
													}
													onBarClick(t.label, section.name);
												}}
											>
												{t.childrenCount > 1 && (
													<TimingBlocks size={size} count={t.childrenCount} />
												)}
											</Bar>
										);
									})}
							</div>
						);
					})}

					<XGrid columns={ticks.length} />
				</div>
			</div>
		</div>
	);
};

// When displaying the chart we must consider the time intervals to display the
// data. For example, if the total time is 10 seconds, we should display the
// data in 200ms intervals. However, if the total time is 1 minute, we should
// display the data in 5 seconds intervals. To achieve this, we define the
// dimensions object that contains the time intervals for the chart.
const tickSpacings = [100, 500, 5_000];

const calcTickSpacing = (totalTime: number): number => {
	const spacings = tickSpacings.slice().reverse();
	for (const s of spacings) {
		if (totalTime > s) {
			return s;
		}
	}
	return spacings[0];
};

// Ensures the sidebar label remains vertically aligned with its corresponding bar.
const applyBarHeightToLabel = (bar: HTMLDivElement | null) => {
	if (!bar) {
		return;
	}
	const labelId = bar.getAttribute("aria-labelledby");
	if (!labelId) {
		return;
	}
	// Selecting a label with special characters (e.g.,
	// #coder_metadata.container_info[0]) will fail because it is not a valid
	// selector. To handle this, we need to query by the id attribute and escape
	// it with quotes.
	const label = document.querySelector<HTMLSpanElement>(
		`[id="${encodeURIComponent(labelId)}"]`,
	);
	if (!label) {
		return;
	}
	label.style.height = `${bar.clientHeight}px`;
};

const durationTime = (duration: Duration): number => {
	return duration.endedAt.getTime() - duration.startedAt.getTime();
};

// Combine multiple durations into a single duration by using the initial start
// time and the final end time.
export const duration = (durations: readonly Duration[]): Duration => {
	const sortedDurations = durations
		.slice()
		.sort((a, b) => a.startedAt.getTime() - b.startedAt.getTime());
	const start = sortedDurations[0].startedAt;

	const sortedEndDurations = durations
		.slice()
		.sort((a, b) => a.endedAt.getTime() - b.endedAt.getTime());
	const end = sortedEndDurations[sortedEndDurations.length - 1].endedAt;
	return { startedAt: start, endedAt: end };
};

const styles = {
	chart: {
		display: "flex",
		alignItems: "stretch",
		height: "100%",
		fontSize: 12,
		fontWeight: 500,
	},
	sidebar: {
		width: columnWidth,
		flexShrink: 0,
		padding: `${XAxisHeight}px 16px`,
	},
	caption: (theme) => ({
		height: YAxisCaptionHeight,
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
		height: "fit-content",
		minHeight: "100%",
	}),
	content: {
		flex: 1,
		position: "relative",
	},
	bars: {
		display: "flex",
		flexDirection: "column",
		gap: barsSpacing,
		padding: `${YAxisCaptionHeight}px ${contentSidePadding}px`,
	},
} satisfies Record<string, Interpolation<Theme>>;
