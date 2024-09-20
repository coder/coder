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
	intervalDimension,
	XAxisHeight,
} from "./constants";
import { Bar } from "./Bar";

export type ChartProps = {
	data: DataSection[];
	onBarClick: (label: string, section: string) => void;
};

// This chart can split data into sections. Eg. display the provisioning timings
// in one section and the scripting time in another
type DataSection = {
	name: string;
	timings: Timing[];
};

// Useful to perform chart operations without requiring additional information
// such as labels or counts, which are only used for display purposes.
export type Duration = {
	startedAt: Date;
	endedAt: Date;
};

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
	count: number;
};

export const Chart: FC<ChartProps> = ({ data, onBarClick }) => {
	const totalDuration = calcTotalDuration(data.flatMap((d) => d.timings));
	const intervals = createIntervals(totalDuration, intervalDimension);

	return (
		<div css={styles.chart}>
			<YAxis>
				{data.map((section) => (
					<YAxisSection key={section.name}>
						<YAxisCaption>{section.name}</YAxisCaption>
						<YAxisLabels>
							{section.timings.map((t) => (
								<YAxisLabel key={t.label} id={`${t.label}-label`}>
									{t.label}
								</YAxisLabel>
							))}
						</YAxisLabels>
					</YAxisSection>
				))}
			</YAxis>

			<div css={styles.main}>
				<XAxis labels={intervals.map(formatAsTimer)} />
				<div css={styles.content}>
					{data.map((section) => {
						return (
							<div key={section.name} css={styles.bars}>
								{section.timings.map((t) => {
									// The time this timing started relative to the initial timing
									const offset = diffInSeconds(
										t.startedAt,
										totalDuration.startedAt,
									);
									const size = secondsToPixel(durationToSeconds(t));
									return (
										<Bar
											key={t.label}
											x={secondsToPixel(offset)}
											width={size}
											afterLabel={`${durationToSeconds(t).toFixed(2)}s`}
											aria-labelledby={`${t.label}-label`}
											ref={applyBarHeightToLabel}
											disabled={t.count <= 1}
											onClick={() => {
												onBarClick(t.label, section.name);
											}}
										>
											{t.count > 1 && (
												<TimingBlocks size={size} count={t.count} />
											)}
										</Bar>
									);
								})}
							</div>
						);
					})}

					<XGrid columns={intervals.length} />
				</div>
			</div>
		</div>
	);
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
	const label = document.querySelector<HTMLSpanElement>(`[id="${labelId}"]`);
	if (!label) {
		return;
	}
	label.style.height = `${bar.clientHeight}px`;
};

// Format a number in seconds to 00:00:00 format
const formatAsTimer = (seconds: number): string => {
	const hours = Math.floor(seconds / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	const remainingSeconds = seconds % 60;

	return `${hours.toString().padStart(2, "0")}:${minutes
		.toString()
		.padStart(2, "0")}:${remainingSeconds.toString().padStart(2, "0")}`;
};

const durationToSeconds = (duration: Duration): number => {
	return (duration.endedAt.getTime() - duration.startedAt.getTime()) / 1000;
};

// Create the intervals to be used in the XAxis
const createIntervals = (duration: Duration, range: number): number[] => {
	const intervals = Math.ceil(durationToSeconds(duration) / range);
	return Array.from({ length: intervals }, (_, i) => i * range + range);
};

const secondsToPixel = (seconds: number): number => {
	return (columnWidth * seconds) / intervalDimension;
};

// Combine multiple durations into a single duration by using the initial start
// time and the final end time.
export const calcTotalDuration = (durations: readonly Duration[]): Duration => {
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

const diffInSeconds = (b: Date, a: Date): number => {
	return (b.getTime() - a.getTime()) / 1000;
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
