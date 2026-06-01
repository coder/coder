// Type-aware height model for the windowing renderer. Pure mutable state, read
// on render and updated from measurement. No DOM or React here.

export type MessageKind = "user" | "assistant" | "tool" | "diff" | "other";

export type HeightCache = {
	// get returns the measured height for an id, or undefined if never measured.
	get(id: string): number | undefined;
	// estimate returns the measured height if known, else the running average for
	// the kind, else the seed. Used to size not-yet-measured items.
	estimate(id: string, kind: MessageKind): number;
	// record stores an id's measured height and folds it into the kind average.
	record(id: string, kind: MessageKind, height: number): void;
};

// Seeds only affect the first paint of a kind before any of its messages have
// been measured. They are intentionally rough.
const DEFAULT_SEEDS: Record<MessageKind, number> = {
	user: 80,
	assistant: 220,
	tool: 140,
	diff: 320,
	other: 160,
};

export function createHeightCache(
	seeds?: Partial<Record<MessageKind, number>>,
): HeightCache {
	const seed: Record<MessageKind, number> = { ...DEFAULT_SEEDS, ...seeds };
	const measured = new Map<string, { kind: MessageKind; height: number }>();
	const totals = new Map<MessageKind, { sum: number; count: number }>();

	const kindAverage = (kind: MessageKind): number | undefined => {
		const total = totals.get(kind);
		if (!total || total.count === 0) {
			return undefined;
		}
		return total.sum / total.count;
	};

	return {
		get(id) {
			return measured.get(id)?.height;
		},
		estimate(id, kind) {
			return measured.get(id)?.height ?? kindAverage(kind) ?? seed[kind];
		},
		record(id, kind, height) {
			const previous = measured.get(id);
			// Only an id's first measurement feeds the kind average. A later
			// re-measurement, such as a streaming item growing token by token,
			// updates just that id's height. Folding it back into the average would
			// drift the estimate of never-measured items above the viewport and
			// shift the reading position while the user reads during a stream.
			if (!previous) {
				const total = totals.get(kind) ?? { sum: 0, count: 0 };
				total.sum += height;
				total.count += 1;
				totals.set(kind, total);
			}
			measured.set(id, { kind, height });
		},
	};
}
