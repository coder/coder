/**
 * @todo What's left to do here:
 * 1. Fill out all incomplete methods
 * 2. Make sure the class respects the resyncOnNewSubscription option
 * 3. Add tests
 */
export const TARGET_REFRESH_ONE_SECOND = 1_000;
export const TARGET_REFRESH_ONE_MINUTE = 60 * 1_000;
export const TARGET_REFRESH_ONE_HOUR = 60 * 60 * 1_000;
export const TARGET_REFRESH_ONE_DAY = 24 * 60 * 60 * 1_000;

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
	#subscriptions: SubscriptionEntry[];
	#latestIntervalId: number | undefined;

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
		this.#subscriptions = [];
		this.#latestIntervalId = undefined;
	}

	#reconcileRefreshIntervals(): void {
		if (this.#subscriptions.length === 0) {
			this.#clearInterval(this.#latestIntervalId);
			return;
		}

		const prevFastestInterval =
			this.#subscriptions[0]?.targetRefreshInterval ?? Number.POSITIVE_INFINITY;
		if (this.#subscriptions.length > 1) {
			this.#subscriptions.sort(
				(e1, e2) => e1.targetRefreshInterval - e2.targetRefreshInterval,
			);
		}

		const newFastestInterval =
			this.#subscriptions[0]?.targetRefreshInterval ?? Number.POSITIVE_INFINITY;
		if (prevFastestInterval === newFastestInterval) {
			return;
		}
		if (newFastestInterval === Number.POSITIVE_INFINITY) {
			this.#clearInterval(this.#latestIntervalId);
			return;
		}

		/**
		 * @todo Figure out the conditions when the interval should be set up,
		 * and when/how it should be updated
		 */
		this.#latestIntervalId = this.#setInterval(() => {
			this.#latestDateSnapshot = this.#createNewDatetime(
				this.#latestDateSnapshot,
			);
			this.#flushUpdateToSubscriptions();
		}, newFastestInterval);
	}

	#flushUpdateToSubscriptions(): void {
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
		this.#reconcileRefreshIntervals();
	}

	subscribe(entry: SubscriptionEntry): void {
		if (entry.targetRefreshInterval <= 0) {
			throw new Error(
				`Refresh interval ${entry.targetRefreshInterval} must be a positive integer (or Infinity)`,
			);
		}

		const subIndex = this.#subscriptions.findIndex((s) => s.id === entry.id);
		if (subIndex === -1) {
			this.#subscriptions.push(entry);
			this.#reconcileRefreshIntervals();
			return;
		}

		const prev = this.#subscriptions[subIndex];
		if (prev === undefined) {
			throw new Error("Went out of bounds");
		}

		this.#subscriptions[subIndex] = entry;
		if (prev.targetRefreshInterval !== entry.targetRefreshInterval) {
			this.#reconcileRefreshIntervals();
		}
	}
}
