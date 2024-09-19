export type Timing = {
	started_at: string;
	ended_at: string;
};

/**
 * Returns the total duration of the timings in seconds.
 */
export const totalDuration = (timings: readonly Timing[]): number => {
	const sortedTimings = timings
		.slice()
		.sort(
			(a, b) =>
				new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
		);
	const start = new Date(sortedTimings[0].started_at);

	const sortedEndTimings = timings
		.slice()
		.sort(
			(a, b) => new Date(a.ended_at).getTime() - new Date(b.ended_at).getTime(),
		);
	const end = new Date(sortedEndTimings[sortedEndTimings.length - 1].ended_at);

	return (end.getTime() - start.getTime()) / 1000;
};

/**
 * Returns an array of intervals in seconds based on the duration.
 */
export const intervals = (duration: number, interval: number): number[] => {
	const intervals = Math.ceil(duration / interval);
	return Array.from({ length: intervals }, (_, i) => i * interval + interval);
};

/**
 * Consolidates the timings into a single timing.
 */
export const consolidateTimings = (timings: readonly Timing[]): Timing => {
	const sortedTimings = timings
		.slice()
		.sort(
			(a, b) =>
				new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
		);
	const start = new Date(sortedTimings[0].started_at);

	const sortedEndTimings = timings
		.slice()
		.sort(
			(a, b) => new Date(a.ended_at).getTime() - new Date(b.ended_at).getTime(),
		);
	const end = new Date(sortedEndTimings[sortedEndTimings.length - 1].ended_at);

	return { started_at: start.toISOString(), ended_at: end.toISOString() };
};

/**
 * Returns the start offset in seconds
 */
export const startOffset = (base: Timing, timing: Timing): number => {
	const parentStart = new Date(base.started_at).getTime();
	const start = new Date(timing.started_at).getTime();
	return (start - parentStart) / 1000;
};
