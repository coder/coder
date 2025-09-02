export const noOp = (..._: readonly unknown[]): void => {};

/**
 * All of TimeSync is basically centered around managing a single Date value,
 * keeping it updated, and notifying subscribers of changes over time. If even a
 * single mutation slips through, that has a risk of breaking everything. So for
 * correctness guarantees, we have to prevent runtime mutations and can't just
 * hope that lying about the types does enough.
 *
 * Using Object.freeze would typically be enough, but Date objects have a lot of
 * set methods that can bypass this restriction and modify internal state. A
 * proxy wrapper is literally our only option.
 */
const readonlyEnforcer: ProxyHandler<Date> = {
	get: (date, key) => {
		if (typeof key === "string" && key.startsWith("set")) {
			return noOp;
		}
		return date[key as keyof Date];
	},
};

// Returns a Date that cannot be modified at runtime. All set methods are turned
// into no-ops. This function does not use a custom type to make it easier to
// interface with existing time libraries.
function newReadonlyDate(sourceDate?: Date): Date {
	const newDate = sourceDate ? new Date(sourceDate) : new Date();
	return new Proxy(newDate, readonlyEnforcer);
}

export type TimeSyncInitOptions = Readonly<{
	/**
	 * The Date value to use when initializing a TimeSync instance.
	 */
	initialDatetime: Date;

	/**
	 * The minimum refresh interval (in milliseconds) to use when dispatching
	 * interval-based state updates.
	 *
	 * If a value smaller than this is specified when trying to set up a new
	 * subscription, this minimum will be used instead.
	 *
	 * It is highly recommended that you only modify this value if you have a
	 * good reason. Updating this value to be too low and make the event loop
	 * get really hot and really tank performance elsewhere in the app.
	 *
	 * If a value of `Number.POSITIVE_INFINITY` is passed in, that renders the
	 * TimeSync completely inert, and no subscriptions will ever be notified.
	 * This behavior can be helpful when setting up snapshot tests.
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
	 * 1. A - Needs to be updated every 500ms
	 * 2. B – Needs to be updated every 1500ms
	 * 3. C – Uses update interval of Infinity
	 *
	 * A, B, and C will all be updated at a rate of 500ms. If A unsubscribes,
	 * then B and C will shift to being updated every 1500ms. If B unsubscribes,
	 * updates will pause completely.
	 */
	targetRefreshIntervalMs: number;
	onUpdate: OnUpdate;
}>;

export const defaultOptions: TimeSyncInitOptions = {
	initialDatetime: new Date(),
	minimumRefreshIntervalMs: 100,
};

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
	 * @throws {RangeError} If the provided interval is not a positive integer.
	 * @returns An unsubscribe callback. Calling the callback more than once
	 * results in a no-op.
	 */
	subscribe: (sh: SubscriptionHandshake) => () => void;

	/**
	 * Allows any system to pull the newest time state from TimeSync, regardless
	 * of whether the system is subscribed.
	 */
	getStateSnapshot: () => Date;

	/**
	 * Immediately tries to refresh the current date snapshot, regardless of
	 * which refresh intervals have been specified. All subscribers will be
	 * notified if the state changed.
	 */
	invalidateStateSnapshot: () => void;

	/**
	 * Cleans up the TimeSync instance and renders it inert for all other
	 * operations.
	 */
	dispose: () => void;
}

type SubscriptionEntry = Readonly<{
	targetRefreshInterval: number;
	unsubscribe: () => void;
}>;

function orderEntries(e1: SubscriptionEntry, e2: SubscriptionEntry): number {
	return e2.targetRefreshInterval - e1.targetRefreshInterval;
}

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
	#disposed: boolean;
	#latestDateSnapshot: Date;

	// Each map value is the list of all refresh intervals actively associated
	// with an onUpdate callback (allowing for duplicate intervals if multiple
	// subscriptions were set up with the exact same onUpdate-interval pair).
	// Each map value should also stay sorted based on refresh interval, in
	// ascending order.
	#subscriptions: Map<OnUpdate, SubscriptionEntry[]>;

	// A cached version of the fastest interval currently registered with
	// TimeSync. Should always be derived from #subscriptions
	#fastestRefreshInterval: number;

	// This class uses setInterval for both its intended purpose and as a janky
	// version of setTimeout. There are a few times when we need timeout-like
	// logic, but if we use setInterval for everything, we have fewer overall
	// data dependencies to mock out and don't have to juggle different IDs
	#latestIntervalId: number | undefined;

	constructor(options?: Partial<TimeSyncInitOptions>) {
		const {
			initialDatetime = defaultOptions.initialDatetime,
			minimumRefreshIntervalMs = defaultOptions.minimumRefreshIntervalMs,
		} = options ?? {};

		const isMinValid =
			minimumRefreshIntervalMs === Number.POSITIVE_INFINITY ||
			(Number.isInteger(minimumRefreshIntervalMs) &&
				minimumRefreshIntervalMs > 0);
		if (!isMinValid) {
			throw new Error(
				`Minimum refresh interval must be positive infinity or a positive integer (received ${minimumRefreshIntervalMs}ms)`,
			);
		}

		this.#disposed = false;
		this.#subscriptions = new Map();
		this.#minimumRefreshIntervalMs = minimumRefreshIntervalMs;
		this.#latestDateSnapshot = newReadonlyDate(initialDatetime);
		this.#fastestRefreshInterval = Number.POSITIVE_INFINITY;
		this.#latestIntervalId = undefined;
	}

	#notifyAllSubscriptions(): void {
		if (this.#disposed) {
			return;
		}

		// We still need to let the logic go through if the current fastest
		// interval is Infinity, so that we can support invalidating the date
		// from public methods
		const subscriptionsPaused =
			this.#minimumRefreshIntervalMs === Number.POSITIVE_INFINITY ||
			this.#subscriptions.size === 0;
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
	 * Updates the cached version of the fastest refresh interval.
	 * @returns {boolean} Indicates whether the new fastest interval was changed
	 */
	#updateFastestInterval(): boolean {
		const prevFastest = this.#fastestRefreshInterval;
		let newFastest = Number.POSITIVE_INFINITY;

		// This setup requires that every interval array stay sorted. It
		// immediately falls apart if this isn't guaranteed.
		for (const entries of this.#subscriptions.values()) {
			const subFastest =
				entries[0]?.targetRefreshInterval ?? Number.POSITIVE_INFINITY;
			if (subFastest < newFastest) {
				newFastest = subFastest;
			}
		}

		this.#fastestRefreshInterval = newFastest;
		return prevFastest !== newFastest;
	}

	#onFastestIntervalChange(): void {
		const fastest = this.#fastestRefreshInterval;
		if (fastest === Number.POSITIVE_INFINITY) {
			window.clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = undefined;
		}

		const elapsed = Date.now() - this.#latestDateSnapshot.getMilliseconds();
		const delta = fastest - elapsed;

		// If we're behind on updates, we need to sync immediately before
		// setting up the new interval. With how TimeSync is designed, this case
		// should only be triggered in response to adding a subscription, never
		// from removing one
		if (delta <= 0) {
			window.clearInterval(this.#latestIntervalId);
			this.#flushUpdate();
			this.#latestIntervalId = window.setInterval(this.#flushUpdate, fastest);
			return;
		}

		window.clearInterval(this.#latestIntervalId);
		this.#latestIntervalId = window.setInterval(() => {
			window.clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = window.setInterval(this.#flushUpdate, fastest);
		}, delta);
	}

	/**
	 * Attempts to update the current Date snapshot, no questions asked.
	 * @returns {boolean} Indicates whether the state actually changed.
	 */
	#updateDateSnapshot(): boolean {
		const newSnap = newReadonlyDate();
		const noStateChange =
			newSnap.getMilliseconds() === this.#latestDateSnapshot.getMilliseconds();
		if (noStateChange) {
			return false;
		}

		this.#latestDateSnapshot = newSnap;
		return true;
	}

	/**
	 * Immediately updates TimeSync to a new snapshot and notifies all
	 * subscribers.
	 *
	 * Defined as an arrow function so that we can just pass it directly to
	 * setInterval without needing to make a new wrapper function each time. We
	 * don't have many situations where we can lose the `this` context, but this
	 * is one of them
	 */
	#flushUpdate = (): void => {
		if (this.#disposed) {
			return;
		}

		const updated = this.#updateDateSnapshot();
		if (updated) {
			this.#notifyAllSubscriptions();
		}
	};

	#removeSubscription(targetRefreshInterval: number, onUpdate: OnUpdate): void {
		const entries = this.#subscriptions.get(onUpdate);
		if (entries === undefined) {
			return;
		}
		const firstMatchIndex = entries.findIndex(
			(e) => e.targetRefreshInterval === targetRefreshInterval,
		);
		if (firstMatchIndex === -1) {
			return;
		}

		// No need to sort on removal because everything gets sorted as it
		// enters the object
		entries.splice(firstMatchIndex, 1);
		if (entries.length === 0) {
			this.#subscriptions.delete(onUpdate);
		}

		const changed = this.#updateFastestInterval();
		if (changed) {
			this.#onFastestIntervalChange();
		}
	}

	#addSubscription(
		targetRefreshInterval: number,
		onUpdate: OnUpdate,
		unsubscribe: () => void,
	): void {
		const initialSubs = this.#subscriptions.size;
		let entries = this.#subscriptions.get(onUpdate);
		if (entries === undefined) {
			entries = [];
			this.#subscriptions.set(onUpdate, entries);
		}

		entries.push({ targetRefreshInterval, unsubscribe });
		if (entries.length > 1) {
			entries.sort(orderEntries);
		}

		const changed = this.#updateFastestInterval();
		if (changed) {
			this.#onFastestIntervalChange();
		}

		// Even if the fastest interval hasn't changed, we should still update
		// the snapshot after the very first subscription gets added. We don't
		// know how much time will have passed between the class getting
		// instantiated and the first subscription
		if (initialSubs === 0) {
			this.#flushUpdate();
		}
	}

	subscribe(sh: SubscriptionHandshake): () => void {
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

		const floored = Math.max(
			this.#minimumRefreshIntervalMs,
			targetRefreshIntervalMs,
		);

		// Because of how subscriptions are represented internally, there's
		// a risk that calling #removeSubscription multiple times could remove
		// subscriptions for completely different systems. So we have to make
		// sure that calls 2+ turn into no-ops
		let unsubscribed = false;

		// Also defining the unsubscribe first, so that we avoid some chicken-
		// and-the-egg problems when adding a subscription
		const unsubscribe = () => {
			if (unsubscribed) {
				return;
			}
			this.#removeSubscription(floored, onUpdate);
			unsubscribed = true;
		};

		this.#addSubscription(floored, onUpdate, unsubscribe);
		return unsubscribe;
	}

	getStateSnapshot(): Date {
		// Some other APIs (like React's useSyncExternalStore) take an approach
		// of getting the latest snapshot first before setting up a
		// subscription. Because we can't actually tell how much time will have
		// elapsed between TimeSync being instantiated and the first
		// subscription being made, the best approach is to do some hidden
		// invalidation when the getter is called
		const isPotentiallyStale =
			this.#subscriptions.size === 0 ||
			this.#fastestRefreshInterval === Number.POSITIVE_INFINITY;

		if (isPotentiallyStale) {
			const shouldInvalidate =
				newReadonlyDate().getTime() - this.#latestDateSnapshot.getTime() >
				this.#minimumRefreshIntervalMs;
			if (shouldInvalidate) {
				this.#flushUpdate();
			}
		}

		return this.#latestDateSnapshot;
	}

	invalidateStateSnapshot(): void {
		this.#flushUpdate();
	}

	dispose(): void {
		if (this.#disposed) {
			return;
		}

		this.#disposed = true;
		window.clearInterval(this.#latestIntervalId);
		for (const entries of this.#subscriptions.values()) {
			for (const e of entries) {
				e.unsubscribe();
			}
		}
		this.#subscriptions.clear();
	}
}
