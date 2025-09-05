export const noOp = (..._: readonly unknown[]): void => {};

/**
 * All of TimeSync is basically centered around managing a single Date value,
 * keeping it updated, and notifying subscribers of changes over time. If even a
 * single mutation slips through, that has a risk of breaking everything. So for
 * correctness guarantees, we have to prevent runtime mutations and can't just
 * hope that lying to TypeScript about the types is enough.
 *
 * Date objects have a lot of private state that can be modified via its set
 * methods, so Object.freeze doesn't do anything to help us.
 */
const readonlyEnforcer: ProxyHandler<Date> = {
	get: (date, key) => {
		if (typeof key === "string" && key.startsWith("set")) {
			return noOp;
		}
		return date[key as keyof Date];
	},
};

/**
 * Returns a Date that cannot be modified at runtime (all set methods still
 * exist, but are turned into no-ops).
 *
 * This function does not use a custom type to make it easier to interface with
 * existing time libraries.
 */
export function newReadonlyDate(sourceDate?: Date): Date {
	const newDate = sourceDate ? new Date(sourceDate) : new Date();
	return new Proxy(newDate, readonlyEnforcer);
}

/**
 * Mainly used here to guarantee no mantissa problems when doing math on
 * intervals but to be on the safe side, we can use this anywhere that doesn't
 * check if a value is equal to the Number.POSITIVE_INFINITY constant
 */
function areIntervalsEqual(interval1: number, interval2: number): boolean {
	const epsilonThreshold = 0.001;
	return Math.abs(interval1 - interval2) < epsilonThreshold;
}

type TimeSyncInitOptions = Readonly<{
	/**
	 * The Date object to initialize TimeSync with to help with snapshot tests.
	 * If this value is specified, the TimeSync instance will be 100% frozen
	 * and will not ever update its state after initialization.
	 */
	snapshotDate: Date;

	/**
	 * The minimum refresh interval (in milliseconds) to use when dispatching
	 * interval-based state updates. Defaults to 200ms.
	 *
	 * If a value smaller than this is specified when trying to set up a new
	 * subscription, this minimum will be used instead.
	 *
	 * It is highly recommended that you only modify this value if you have a
	 * good reason. Updating this value to be too low and make the event loop
	 * get really hot and really tank performance elsewhere in the app.
	 */
	minimumRefreshIntervalMs: number;
}>;

/**
 * The callback to call when a new state update is ready to be dispatched.
 */
export type OnUpdate = (newDate: Date) => void;

export type SubscriptionHandshake = Readonly<{
	/**
	 * The maximum update interval that a subscriber needs. A value of
	 * Number.POSITIVE_INFINITY indicates that the subscriber does not strictly
	 * need any updates (though they may still happen based on other
	 * subscribers).
	 *
	 * TimeSync always dispatches updates based on the lowest update interval
	 * among all subscribers.
	 *
	 * For example, let's say that we have these three subscribers:
	 * 1. A - Needs updates no slower than 500ms
	 * 2. B – Needs updates no slower than 1000ms
	 * 3. C – Uses interval of Infinity (does not strictly need an update)
	 *
	 * A, B, and C will all be updated at a rate of 500ms. If A unsubscribes,
	 * then B and C will shift to being updated every 1000ms. If B unsubscribes,
	 * updates will pause completely.
	 */
	targetRefreshIntervalMs: number;
	onUpdate: OnUpdate;
}>;

type InvalidateSnapshotOptions = Readonly<{
	notificationBehavior?: "onChange" | "never" | "always";
}>;

interface TimeSyncApi {
	/**
	 * Subscribes an external system to TimeSync.
	 *
	 * The same callback (by reference) is allowed to be registered multiple
	 * times, either for the same update interval, or different update
	 * intervals. However, while each subscriber is tracked individually, when
	 * a new state update needs to be dispatched, the onUpdate callback will be
	 * called once, total.
	 *
	 * @throws {RangeError} If the provided interval is not either a positive
	 * integer or positive infinity.
	 * @returns An unsubscribe callback. Calling the callback more than once
	 * results in a no-op.
	 */
	subscribe: (sh: SubscriptionHandshake) => () => void;

	/**
	 * Allows any system to pull the latest time state from TimeSync, regardless
	 * of whether the system is subscribed.
	 *
	 * @returns The Date produced from the most recent internal update.
	 */
	getStateSnapshot: () => Date;

	/**
	 * Immediately tries to refresh the current date snapshot, regardless of
	 * which refresh intervals have been specified.
	 *
	 * @returns The date state post-invalidation (which might be the same as
	 * before).
	 */
	invalidateStateSnapshot: (options: InvalidateSnapshotOptions) => Date;

	/**
	 * Cleans up the TimeSync instance and renders it inert for all other
	 * operations.
	 */
	dispose: () => void;
}

type SubscriptionEntry = Readonly<{
	targetInterval: number;
	unsubscribe: () => void;
}>;

const defaultMinimumRefreshIntervalMs: number = 200;

/**
 * TimeSync provides a centralized authority for working with time values in a
 * more structured way, where all dependents for the time values must stay in
 * sync with each other. (e.g., in a React codebase, you want multiple
 * components that rely on time values to update together, to avoid screen
 * tearing and stale data for only some parts of the screen).
 *
 * By design, there is no way to let a subscriber disable updates. That defeats
 * the goal of needing to keep everything in sync with each other. If updates
 * are happening too frequently in React, restructure how you're composing your
 * components to minimize the costs of re-renders.
 *
 * See comments for exported methods and types for more information.
 */
export class TimeSync implements TimeSyncApi {
	readonly #minimumRefreshIntervalMs: number;
	readonly #isFrozen: boolean;
	#isDisposed: boolean;
	#latestDateSnapshot: Date;

	// Stores all refresh intervals actively associated with an onUpdate
	// callback (along with their associated unsubscribe callbacks). "Duplicate"
	// intervals are allowed (in case multiple systems subscribe with the same
	// interval-onUpdate pairs). Each map value should stay sorted by refresh
	// interval, in ascending order.
	#subscriptions: Map<OnUpdate, SubscriptionEntry[]>;

	// A cached version of the fastest interval currently registered with
	// TimeSync. Should always be derived from #subscriptions
	#fastestRefreshInterval: number;

	// This class uses setInterval for both its intended purpose and as a janky
	// version of setTimeout. There are a few times when we need timeout-like
	// logic, but if we use setInterval for everything, we have fewer IDs to
	// juggle, and less risk of things getting out of sync. Type defined like
	// this to support client-rendering and non-RSC server-rendering
	#intervalId: NodeJS.Timeout | number | undefined;

	constructor(options?: Partial<TimeSyncInitOptions>) {
		const {
			snapshotDate,
			minimumRefreshIntervalMs = defaultMinimumRefreshIntervalMs,
		} = options ?? {};

		const isMinValid =
			Number.isInteger(minimumRefreshIntervalMs) &&
			minimumRefreshIntervalMs > 0;
		if (!isMinValid) {
			throw new Error(
				`Minimum refresh interval must be a positive integer (received ${minimumRefreshIntervalMs}ms)`,
			);
		}

		this.#subscriptions = new Map();
		this.#minimumRefreshIntervalMs = minimumRefreshIntervalMs;
		this.#isDisposed = false;
		this.#isFrozen = snapshotDate !== undefined;
		this.#latestDateSnapshot = newReadonlyDate(snapshotDate);
		this.#fastestRefreshInterval = Number.POSITIVE_INFINITY;
		this.#intervalId = undefined;
	}

	#notifyAllSubscriptions(): void {
		// We still need to let the logic go through if the current fastest
		// interval is Infinity, so that we can support letting any arbitrary
		// consumer invalidate the date immediately
		const subscriptionsPaused =
			this.#isDisposed || this.#isFrozen || this.#subscriptions.size === 0;
		if (subscriptionsPaused) {
			return;
		}

		// Copying the latest state into a separate variable, just to make
		// absolutely sure that if the `this` context magically changes between
		// callback calls (e.g., one of the subscribers calling the invalidate
		// method), it doesn't cause subscribers to receive different values.
		const bound = this.#latestDateSnapshot;

		// While this is a super niche use case, we're actually safe if a
		// subscriber disposes of the whole TimeSync instance. Once the Map is
		// cleared, the map's iterator will automatically break the loop. So
		// there's no risk of continuing to dispatch values after cleanup.
		for (const onUpdate of this.#subscriptions.keys()) {
			onUpdate(bound);
		}
	}

	/**
	 * The logic that should happen at each step in TimeSync's active interval.
	 *
	 * Defined as an arrow function so that we can just pass it directly to
	 * setInterval without needing to make a new wrapper function each time. We
	 * don't have many situations where we can lose the `this` context, but this
	 * is one of them
	 */
	#onTick = (): void => {
		if (this.#isDisposed || this.#isFrozen) {
			// Defensive step to make sure that an invalid tick wasn't started
			clearInterval(this.#intervalId);
			this.#intervalId = undefined;
			return;
		}

		const updated = this.#updateDateSnapshot();
		if (updated) {
			this.#notifyAllSubscriptions();
		}
	};

	#onFastestIntervalChange(): void {
		const fastest = this.#fastestRefreshInterval;
		const skipUpdate =
			this.#isDisposed ||
			this.#isFrozen ||
			fastest === Number.POSITIVE_INFINITY;
		if (skipUpdate) {
			clearInterval(this.#intervalId);
			this.#intervalId = undefined;
			return;
		}

		const elapsed =
			newReadonlyDate().getTime() - this.#latestDateSnapshot.getMilliseconds();
		const timeBeforeNextUpdate = fastest - elapsed;

		if (timeBeforeNextUpdate <= 0) {
			clearInterval(this.#intervalId);
			const updated = this.#updateDateSnapshot();
			if (updated) {
				this.#notifyAllSubscriptions();
			}
			this.#intervalId = setInterval(this.#onTick, fastest);
			return;
		}

		clearInterval(this.#intervalId);
		this.#intervalId = setInterval(() => {
			clearInterval(this.#intervalId);
			this.#intervalId = setInterval(this.#onTick, fastest);
		}, timeBeforeNextUpdate);
	}

	#updateFastestInterval(): void {
		if (this.#isDisposed || this.#isFrozen) {
			this.#fastestRefreshInterval = Number.POSITIVE_INFINITY;
			return;
		}

		const prevFastest = this.#fastestRefreshInterval;
		let newFastest = Number.POSITIVE_INFINITY;

		// This setup requires that every interval array stay sorted. It
		// immediately falls apart if this isn't guaranteed.
		for (const entries of this.#subscriptions.values()) {
			const subFastest = entries[0]?.targetInterval ?? Number.POSITIVE_INFINITY;
			if (subFastest < newFastest) {
				newFastest = subFastest;
			}
		}

		this.#fastestRefreshInterval = newFastest;
		if (!areIntervalsEqual(prevFastest, newFastest)) {
			this.#onFastestIntervalChange();
		}
	}

	/**
	 * Attempts to update the current Date snapshot, no questions asked.
	 * @returns {boolean} Indicates whether the state actually changed.
	 */
	#updateDateSnapshot(): boolean {
		if (this.#isDisposed || this.#isFrozen) {
			return false;
		}

		const newSnap = newReadonlyDate();
		const noStateChange = areIntervalsEqual(
			newSnap.getMilliseconds(),
			this.#latestDateSnapshot.getMilliseconds(),
		);
		if (noStateChange) {
			return false;
		}

		this.#latestDateSnapshot = newSnap;
		return true;
	}

	subscribe(sh: SubscriptionHandshake): () => void {
		if (this.#isDisposed || this.#isFrozen) {
			return noOp;
		}

		// Destructuring properties so that they can't be fiddled with after
		// this function call ends
		const { targetRefreshIntervalMs, onUpdate } = sh;

		const isTargetValid =
			targetRefreshIntervalMs === Number.POSITIVE_INFINITY ||
			(Number.isInteger(targetRefreshIntervalMs) &&
				targetRefreshIntervalMs > 0);
		if (!isTargetValid) {
			throw new Error(
				`Target refresh interval must be positive infinity or a positive integer (received ${targetRefreshIntervalMs}ms)`,
			);
		}

		const unsubscribe = (): void => {
			const entries = this.#subscriptions.get(onUpdate);
			if (entries === undefined) {
				return;
			}
			const matchIndex = entries.findIndex(
				(e) => e.unsubscribe === unsubscribe,
			);
			if (matchIndex === -1) {
				return;
			}
			// No need to sort on removal because everything gets sorted as it
			// enters the subscriptions map
			entries.splice(matchIndex, 1);
			if (entries.length === 0) {
				this.#subscriptions.delete(onUpdate);
			}
			this.#updateFastestInterval();
		};

		let entries = this.#subscriptions.get(onUpdate);
		if (entries === undefined) {
			entries = [];
			this.#subscriptions.set(onUpdate, entries);
		}

		const targetInterval = Math.max(
			this.#minimumRefreshIntervalMs,
			targetRefreshIntervalMs,
		);
		entries.push({ unsubscribe, targetInterval });
		entries.sort((e1, e2) => e2.targetInterval - e1.targetInterval);
		this.#updateFastestInterval();

		return unsubscribe;
	}

	getStateSnapshot(): Date {
		return this.#latestDateSnapshot;
	}

	invalidateStateSnapshot(options?: InvalidateSnapshotOptions): Date {
		if (this.#isDisposed || this.#isFrozen) {
			return this.#latestDateSnapshot;
		}

		const { notificationBehavior = "onChange" } = options ?? {};
		const changed = this.#updateDateSnapshot();
		const shouldNotify =
			(changed && notificationBehavior === "onChange") ||
			notificationBehavior === "always";
		if (shouldNotify) {
			this.#notifyAllSubscriptions();
		}
		return this.#latestDateSnapshot;
	}

	dispose(): void {
		if (this.#isDisposed) {
			return;
		}

		this.#isDisposed = true;
		clearInterval(this.#intervalId);
		for (const entries of this.#subscriptions.values()) {
			for (const e of entries) {
				e.unsubscribe();
			}
		}
		this.#subscriptions.clear();
	}
}
