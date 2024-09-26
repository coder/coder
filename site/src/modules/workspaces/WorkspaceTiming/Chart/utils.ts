export type BaseTiming = {
	startedAt: Date;
	endedAt: Date;
};

export const combineTimings = (timings: BaseTiming[]): BaseTiming => {
	// If there are no timings, return a timing with the same start and end
	// times. This prevents the chart from breaking when calculating the start and
	// end times from an empty array.
	if (timings.length === 0) {
		return { startedAt: new Date(), endedAt: new Date() };
	}

	const sortedDurations = timings
		.slice()
		.sort((a, b) => a.startedAt.getTime() - b.startedAt.getTime());
	const start = sortedDurations[0].startedAt;

	const sortedEndDurations = timings
		.slice()
		.sort((a, b) => a.endedAt.getTime() - b.endedAt.getTime());
	const end = sortedEndDurations[sortedEndDurations.length - 1].endedAt;
	return { startedAt: start, endedAt: end };
};

export const calcDuration = (timing: BaseTiming): number => {
	return timing.endedAt.getTime() - timing.startedAt.getTime();
};

// When displaying the chart we must consider the time intervals to display the
// data. For example, if the total time is 10 seconds, we should display the
// data in 200ms intervals. However, if the total time is 1 minute, we should
// display the data in 5 seconds intervals. To achieve this, we define the
// dimensions object that contains the time intervals for the chart.
const scales = [100, 500, 5_000];

const pickScale = (totalTime: number): number => {
	const reversedScales = scales.slice().reverse();
	for (const s of reversedScales) {
		if (totalTime > s) {
			return s;
		}
	}
	return reversedScales[0];
};

export const makeTicks = (time: number) => {
	const scale = pickScale(time);
	const count = Math.ceil(time / scale);
	const ticks = Array.from({ length: count }, (_, i) => i * scale + scale);
	return [ticks, scale] as const;
};

export const formatTime = (time: number): string => {
	return `${time.toLocaleString()}ms`;
};

export const calcOffset = (
	timing: BaseTiming,
	generalTiming: BaseTiming,
): number => {
	return timing.startedAt.getTime() - generalTiming.startedAt.getTime();
};
