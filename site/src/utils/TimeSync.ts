/**
 * @todo What's left to do here:
 * 1. Fill out all incomplete methods
 * 2. Refactor as necessary
 * 3. Add tests and address any bugs
 */
export type SetInterval = (fn: () => void, intervalMs: number) => number;
export type ClearInterval = (id: number | undefined) => void;

export type TimeSyncInitOptions = Readonly<{
	/**
	 * The Date value to use when initializing a TimeSync instance.
	 */
	initialDatetime: Date;

	/**
	 * The function to use when creating a new datetime snapshot when a TimeSync
	 * needs to update on an interval.
	 */
	createNewDatetime: (prevDatetime: Date) => Date;

	/**
	 * The function to use when creating new intervals.
	 */
	setInterval: SetInterval;

	/**
	 * The function to use when clearing intervals.
	 *
	 * (e.g., Clearing a previous interval because the TimeSync needs to make a
	 * new interval to increase/decrease its update speed.)
	 */
	clearInterval: ClearInterval;

	/**
	 * Configures whether adding a new subscription will immediately create
	 * a new time snapshot and use it to update all other subscriptions.
	 */
	resyncOnNewSubscription: boolean;
}>;

export const defaultOptions: TimeSyncInitOptions = {
	initialDatetime: new Date(),
	resyncOnNewSubscription: true,
	createNewDatetime: () => new Date(),
	setInterval: window.setInterval,
	clearInterval: window.clearInterval,
};

export type SubscriptionEntry = Readonly<{
	id: string;
	targetRefreshInterval: number;
	onUpdate: (newDatetime: Date) => void;
}>;

interface TimeSyncApi {
	subscribe: (entry: SubscriptionEntry) => void;
	unsubscribe: (id: string) => void;
	getTimeSnapshot: () => Date;
}

/**
 * TimeSync provides a centralized authority for working with time values in a
 * more structured, "pure function-ish" way, where all dependents for the time
 * values must stay in sync with each other. (e.g., in a React codebase, you
 * want multiple components that rely on time values to update together, to
 * avoid screen tearing and stale data for only some parts of the screen).
 *
 * It lets any number of consumers subscribe to it, requiring that subscribers
 * define the slowest possible update interval they need to receive new time
 * values for. A value of positive Infinity indicates that a subscriber doesn't
 * need updates; if all subscriptions have an update interval of Infinity, the
 * class may not dispatch updates.
 *
 * The class aggregates all the update intervals, and will dispatch updates to
 * all consumers based on the fastest refresh interval needed. (e.g., if
 * subscriber A needs no updates, but subscriber B needs updates every second,
 * BOTH will update every second until subscriber B unsubscribes. After that,
 * TimeSync will stop dispatching updates until subscription C gets added, and C
 * has a non-Infinite update interval).
 *
 * By design, there is no way to make one subscriber disable updates. That
 * defeats the goal of needing to keep everything in sync with each other. If
 * updates are happening too frequently in React, restructure how you're
 * composing your components to minimize the costs of re-renders.
 */
export class TimeSync implements TimeSyncApi {
	readonly #resyncOnNewSubscription: boolean;
	readonly #createNewDatetime: (prev: Date) => Date;
	readonly #setInterval: SetInterval;
	readonly #clearInterval: ClearInterval;

	#latestDateSnapshot: Date;
	#subscriptions: SubscriptionEntry[] = [];
	#latestIntervalId: number | undefined = undefined;
	#currentRefreshInterval: number = Number.POSITIVE_INFINITY;

	constructor(options: Partial<TimeSyncInitOptions>) {
		const {
			initialDatetime = defaultOptions.initialDatetime,
			resyncOnNewSubscription = defaultOptions.resyncOnNewSubscription,
			createNewDatetime = defaultOptions.createNewDatetime,
			setInterval = defaultOptions.setInterval,
			clearInterval = defaultOptions.clearInterval,
		} = options;

		this.#setInterval = setInterval;
		this.#clearInterval = clearInterval;
		this.#createNewDatetime = createNewDatetime;
		this.#resyncOnNewSubscription = resyncOnNewSubscription;
		this.#latestDateSnapshot = initialDatetime;
	}

	#onSubscriptionAdd(): void {
		if (this.#resyncOnNewSubscription) {
			this.#updateSnapshot();
		}

		// Sort by refresh speed, descending
		this.#subscriptions.sort(
			(e1, e2) => e1.targetRefreshInterval - e2.targetRefreshInterval,
		);

		const oldFastestInterval = this.#currentRefreshInterval;
		const newFastestInterval =
			this.#subscriptions[0]?.targetRefreshInterval ?? Number.POSITIVE_INFINITY;
		if (oldFastestInterval === newFastestInterval) {
			return;
		}

		this.#currentRefreshInterval = newFastestInterval;
		this.#clearInterval(this.#latestIntervalId);
		this.#latestIntervalId = undefined;
		if (this.#currentRefreshInterval === Number.POSITIVE_INFINITY) {
			return;
		}

		const newTime = this.#createNewDatetime(this.#latestDateSnapshot);
		const elapsed =
			newTime.getMilliseconds() - this.#latestDateSnapshot.getMilliseconds();

		const startNewIntervalFromScratch = () => {
			this.#latestIntervalId = this.#setInterval(
				() => this.#updateSnapshot(),
				this.#currentRefreshInterval,
			);
		};

		const unfulfilled = Math.max(0, this.#currentRefreshInterval - elapsed);
		if (unfulfilled === 0) {
			startNewIntervalFromScratch();
			return;
		}

		// Feels a bit hokey to be setting a timeout via setInterval, but
		// didn't want to add even more data dependencies to the constructor
		// when setInterval can be used just fine in a pinch
		this.#latestIntervalId = this.#setInterval(() => {
			this.#clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = undefined;
			startNewIntervalFromScratch();
		}, unfulfilled);
	}

	/**
	 * @todo This isn't done yet
	 */
	#onSubscriptionRemoval(): void {
		if (this.#subscriptions.length === 0) {
			this.#clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = undefined;
			this.#currentRefreshInterval = Number.POSITIVE_INFINITY;
			return;
		}

		const newFastestInterval =
			this.#subscriptions[0]?.targetRefreshInterval ?? Number.POSITIVE_INFINITY;
		if (newFastestInterval === this.#currentRefreshInterval) {
			return;
		}
	}

	#updateSnapshot(): void {
		// It's assumed that TimeSync will be used with subscribers that will
		// treat whatever Date they receive as an immutable value. But because
		// the entire class breaks if someone does mutate it, we need to freeze
		// the value
		const newRawDate = this.#createNewDatetime(this.#latestDateSnapshot);
		Object.freeze(newRawDate);
		this.#latestDateSnapshot = newRawDate;

		for (const subEntry of this.#subscriptions) {
			subEntry.onUpdate(this.#latestDateSnapshot);
		}
	}

	getTimeSnapshot(): Date {
		return this.#latestDateSnapshot;
	}

	unsubscribe(id: string): void {
		const updated = this.#subscriptions.filter((s) => s.id !== id);
		if (updated.length === this.#subscriptions.length) {
			return;
		}

		this.#subscriptions = updated;
		this.#onSubscriptionRemoval();
	}

	subscribe(entry: SubscriptionEntry): void {
		const intervalIsValid =
			entry.targetRefreshInterval > 0 &&
			(Number.isInteger(entry.targetRefreshInterval) ||
				entry.targetRefreshInterval === Number.POSITIVE_INFINITY);
		if (!intervalIsValid) {
			throw new Error(
				`Refresh interval ${entry.targetRefreshInterval} must be a positive integer or Positive Infinity)`,
			);
		}

		const subIndex = this.#subscriptions.findIndex((s) => s.id === entry.id);
		if (subIndex === -1) {
			this.#subscriptions.push(entry);
			this.#onSubscriptionAdd();
			return;
		}

		const prev = this.#subscriptions[subIndex];
		if (prev === undefined) {
			throw new Error("Went out of bounds");
		}

		this.#subscriptions[subIndex] = entry;
		if (prev.targetRefreshInterval !== entry.targetRefreshInterval) {
			this.#onSubscriptionAdd();
		}
	}
}
