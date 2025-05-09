export type TimeRange = {
	startedAt: Date;
	endedAt: Date;
};

/**
 * Combines multiple timings into a single timing that spans the entire duration
 * of the input timings.
 */
export const mergeTimeRanges = (ranges: TimeRange[]): TimeRange => {
	const sortedDurations = ranges
		.slice()
		.sort((a, b) => a.startedAt.getTime() - b.startedAt.getTime());
	const start = sortedDurations[0].startedAt;

	const sortedEndDurations = [...ranges].sort(
		(a, b) => a.endedAt.getTime() - b.endedAt.getTime(),
	);
	const end = sortedEndDurations[sortedEndDurations.length - 1].endedAt;
	return { startedAt: start, endedAt: end };
};

export const calcDuration = (range: TimeRange): number => {
	return range.endedAt.getTime() - range.startedAt.getTime();
};

// When displaying the chart we must consider the time intervals to display the
// data. For example, if the total time is 10 seconds, we should display the
// data in 200ms intervals. However, if the total time is 1 minute, we should
// display the data in 5 seconds intervals. To achieve this, we define the
// dimensions object that contains the time intervals for the chart.
const second = 1_000;
const minute = 60 * second;
const hour = 60 * minute;
const day = 24 * hour;
const scales = [
	day,
	hour,
	5 * minute,
	minute,
	10 * second,
	5 * second,
	500,
	100,
];

const pickScale = (totalTime: number): number => {
	for (const s of scales) {
		if (totalTime > s) {
			return s;
		}
	}
	return scales[0];
};

export const makeTicks = (time: number) => {
	const scale = pickScale(time);
	const count = Math.ceil(time / scale);
	const ticks = Array.from({ length: count }, (_, i) => i * scale + scale);
	return [ticks, scale] as const;
};

export const formatTime = (time: number): string => {
	const absTime = Math.abs(time);
	let unit = "";
	let value = 0;

	if (absTime < second) {
		value = time;
		unit = "ms";
	} else if (absTime < minute) {
		value = time / second;
		unit = "s";
	} else if (absTime < hour) {
		value = time / minute;
		unit = "m";
	} else if (absTime < day) {
		value = time / hour;
		unit = "h";
	} else {
		value = time / day;
		unit = "d";
	}
	return `${value.toLocaleString(undefined, {
		maximumFractionDigits: 2,
	})}${unit}`;
};

export const calcOffset = (range: TimeRange, baseRange: TimeRange): number => {
	return range.startedAt.getTime() - baseRange.startedAt.getTime();
};
