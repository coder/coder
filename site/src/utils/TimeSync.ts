/**
 * A readonly version of a Date object. The object is guaranteed to lack all
 * methods that can modify internal state both at compile time and runtime.
 */
export type ReadonlyDate = Omit<Date, `set${string}`>;

const noOp = (..._: readonly unknown[]): void => {};

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

function newReadonlyDate(sourceDate?: Date): ReadonlyDate {
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
	 */
	minimumRefreshIntervalMs: number;

	/**
	 * Configures whether subscribers will be notified immediately after a new
	 * Date snapshot has been created. Defaults to true.
	 *
	 * The main use case for disabling this is for interfacing with systems with
	 * very strict rules for side effects. To minimize stale data issues, adding
	 * a new subscription might immediately update the Date state. If it is NOT
	 * safe to notify all other subscribers right after this, set this property
	 * to false, and then use the `notifySubscribers` method to manage
	 * subscriptions manually.
	 */
	autoNotifyAfterStateUpdate: boolean;
}>;

/**
 * The callback to call when a new state update is ready to be dispatched.
 */
export type OnUpdate = (newDate: ReadonlyDate) => void;

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

const defaultOptions: TimeSyncInitOptions = {
	initialDatetime: new Date(),
	autoNotifyAfterStateUpdate: true,
	minimumRefreshIntervalMs: 100,
};

export type SubscriptionResult = Readonly<{
	/**
	 * Allows an external system to remove a specific subscription. When called,
	 * this only removes a single subscription instance. If other subscribers
	 * have registered the exact same callback, they will not be affected.
	 */
	unsubscribe: () => void;

	/**
	 * Indicates whether a new Date snapshot was created in response to the new
	 * subscription, and whether there are other subscribers that need to be
	 * notified.
	 *
	 * Intended to be used with TimeSync's `notifyAfterUpdate` config property.
	 * If `notifyAfterUpdate` is true, this value will always be false.
	 */
	hasPendingSubscribers: boolean;
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
	 * @throws {RangeError} If the provided interval is not a positive integer.
	 */
	subscribe: (entry: SubscriptionHandshake) => SubscriptionResult;

	/**
	 * Allows any system to pull the newest time state from TimeSync, regardless
	 * of whether the system is subscribed.
	 */
	getStateSnapshot: () => ReadonlyDate;

	/**
	 * Allows any external system to manually flush the latest time snapshot to
	 * all subscribers.
	 *
	 * Does not need to be used if TimeSync has already been configured with
	 * `notifyAfterUpdate` set to true.
	 */
	updateAllSubscriptions: () => void;
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
	readonly #autoNotifyAfterStateUpdate: boolean;
	readonly #minimumRefreshIntervalMs: number;
	#latestDateSnapshot: ReadonlyDate;

	// Each map value is the list of all refresh intervals actively associated
	// with an onUpdate callback (allowing for duplicate intervals if multiple
	// subscriptions were set up with the exact same onUpdate-interval pair).
	// Each map value should also stay sorted in ascending order.
	#subscriptions = new Map<OnUpdate, number[]>();

	// A cached version of the fastest interval currently registered with
	// TimeSync. Should always be derived from #subscriptions
	#fastestRefreshInterval = Number.POSITIVE_INFINITY;

	// This class uses setInterval for both its intended purpose and as a janky
	// version of setTimeout. There are a few times when we need timeout-like
	// logic, but if we use setInterval for everything, we have fewer overall
	// data dependencies to mock out and don't have to juggle different IDs
	#latestIntervalId: number | undefined = undefined;

	constructor(options?: Partial<TimeSyncInitOptions>) {
		const {
			initialDatetime = defaultOptions.initialDatetime,
			autoNotifyAfterStateUpdate = defaultOptions.autoNotifyAfterStateUpdate,
			minimumRefreshIntervalMs = defaultOptions.minimumRefreshIntervalMs,
		} = options ?? {};

		const minIsInvalid =
			!Number.isInteger(minimumRefreshIntervalMs) ||
			minimumRefreshIntervalMs <= 0;
		if (minIsInvalid) {
			throw new Error(
				`Minimum refresh interval must be a positive integer (received ${minimumRefreshIntervalMs}ms)`,
			);
		}

		this.#autoNotifyAfterStateUpdate = autoNotifyAfterStateUpdate;
		this.#minimumRefreshIntervalMs = minimumRefreshIntervalMs;
		this.#latestDateSnapshot = newReadonlyDate(initialDatetime);
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
		for (const intervals of this.#subscriptions.values()) {
			const subFastest = intervals[0] ?? Number.POSITIVE_INFINITY;
			if (subFastest < newFastest) {
				newFastest = subFastest;
			}
		}

		this.#fastestRefreshInterval = newFastest;
		return prevFastest !== newFastest;
	}

	/**
	 * Attempts to update the current Date snapshot.
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
	 * Updates TimeSync to a new snapshot immediately and synchronously.
	 *
	 * @returns {boolean} Indicates whether there are still subscribers that
	 * need to be notified after the update.
	 */
	#flushUpdate(): boolean {
		const hasPendingUpdate = this.#updateDateSnapshot();
		if (hasPendingUpdate && this.#autoNotifyAfterStateUpdate) {
			this.updateAllSubscriptions();
		}
		return hasPendingUpdate && !this.#autoNotifyAfterStateUpdate;
	}

	/**
	 * The callback to wire up directly to setInterval for periodic updates.
	 *
	 * Defined as an arrow function so that we can just pass it directly to
	 * setInterval without needing to make a new wrapper function each time. We
	 * don't have many situations where we can lose the `this` context, but this
	 * is one of them
	 */
	#onTick = (): void => {
		const hasPendingUpdate = this.#updateDateSnapshot();
		if (hasPendingUpdate) {
			this.updateAllSubscriptions();
		}
	};

	#onFastestIntervalChange(): boolean {
		const fastest = this.#fastestRefreshInterval;
		if (fastest === Number.POSITIVE_INFINITY) {
			window.clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = undefined;
			return false;
		}

		const elapsed = Date.now() - this.#latestDateSnapshot.getMilliseconds();
		const delta = fastest - elapsed;

		// If we're behind on updates, we need to sync immediately before
		// setting up the new interval. With how TimeSync is designed, this case
		// should only be triggered in response to adding a subscription, never
		// from removing one
		if (delta <= 0) {
			window.clearInterval(this.#latestIntervalId);
			const hasPendingSubscribers = this.#flushUpdate();
			this.#latestIntervalId = window.setInterval(this.#onTick, fastest);
			return hasPendingSubscribers;
		}

		window.clearInterval(this.#latestIntervalId);
		this.#latestIntervalId = window.setInterval(() => {
			window.clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = window.setInterval(this.#onTick, fastest);
		}, delta);

		return false;
	}

	#addSubscription(targetRefreshInterval: number, onUpdate: OnUpdate): boolean {
		const initialSubs = this.#subscriptions.size;
		let intervals = this.#subscriptions.get(onUpdate);
		if (intervals === undefined) {
			intervals = [];
			this.#subscriptions.set(onUpdate, intervals);
		}

		intervals.push(targetRefreshInterval);
		if (intervals.length > 1) {
			intervals.sort((i1, i2) => i2 - i1);
		}

		let hasPendingSubscribers = false;
		const changed = this.#updateFastestInterval();
		if (changed) {
			hasPendingSubscribers ||= this.#onFastestIntervalChange();
		}

		// Even if the fastest interval hasn't changed, we should still update
		// the snapshot after the very first subscription gets added. We don't
		// know how much time will have passed between the class getting
		// instantiated and the first subscription
		if (initialSubs === 0) {
			hasPendingSubscribers ||= this.#flushUpdate();
		}

		return hasPendingSubscribers;
	}

	#removeSubscription(targetRefreshInterval: number, onUpdate: OnUpdate): void {
		const intervals = this.#subscriptions.get(onUpdate);
		if (intervals === undefined) {
			return;
		}
		const firstMatchIndex = intervals.indexOf(targetRefreshInterval);
		if (firstMatchIndex === -1) {
			return;
		}

		intervals.splice(firstMatchIndex, 1);
		if (intervals.length > 1) {
			intervals.sort((i1, i2) => i2 - i1);
		}
		if (intervals.length === 0) {
			this.#subscriptions.delete(onUpdate);
		}

		const changed = this.#updateFastestInterval();
		if (changed) {
			this.#onFastestIntervalChange();
		}
	}

	subscribe(hs: SubscriptionHandshake): SubscriptionResult {
		// Destructuring properties so that they can't be fiddled with after
		// this function call ends
		const { targetRefreshIntervalMs, onUpdate } = hs;

		const isInputValid =
			Number.isInteger(targetRefreshIntervalMs) && targetRefreshIntervalMs > 0;
		if (!isInputValid) {
			throw new RangeError(
				`TimeSync refresh interval must be a positive integer (received ${targetRefreshIntervalMs}ms)`,
			);
		}

		const floored = Math.max(
			this.#minimumRefreshIntervalMs,
			targetRefreshIntervalMs,
		);
		const hasPendingSubscribers = this.#addSubscription(floored, onUpdate);

		return {
			hasPendingSubscribers,
			unsubscribe: () => {
				this.#removeSubscription(floored, onUpdate);
			},
		};
	}

	getStateSnapshot(): ReadonlyDate {
		return this.#latestDateSnapshot;
	}

	updateAllSubscriptions(): void {
		const subscriptionsPaused =
			this.#subscriptions.size === 0 ||
			this.#fastestRefreshInterval === Number.POSITIVE_INFINITY;
		if (subscriptionsPaused) {
			return;
		}

		// Copying the latest state into a separate variable, just to make
		// absolutely sure that if the `this` context magically changes between
		// callback calls, it doesn't cause subscribers to receive different
		// values.
		const bound = this.#latestDateSnapshot;
		for (const onUpdate of this.#subscriptions.keys()) {
			onUpdate(bound);
		}
	}
}
