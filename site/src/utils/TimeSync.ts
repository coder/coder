/**
 * A readonly version of a Date object. The object lacks all set methods at the
 * type level, and is frozen at runtime.
 */
export type ReadonlyDate = Omit<Date, `set${string}`>;

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
	targetRefreshInterval: number;
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
	 * @throws {RangeError} If the provided interval is less than or equal to 0.
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
	notifySubscribers: () => void;
}

type SubscriptionTracker = Readonly<{
	targetRefreshInterval: number;
	// Each key is an individual onUpdate callback, while each value is the
	// number of subscribers that currently have that callback registered
	updates: Map<OnUpdate, number>;
}>;

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

	/**
	 * @todo Ran out of time for this, but to make sure that the state system is
	 * 100% airtight, we need to make sure that the Date object is truly
	 * readonly at runtime.
	 *
	 * The ReadonlyDate type hides the set methods at compile time, and freezing
	 * the Date objects at runtime helps a little. But the set methods still
	 * exist at runtime, and there's nothing stopping them from modifying the
	 * internal Date object state. A single one of those calls can destroy the
	 * entire state management strategy.
	 *
	 * This can probably be punted for a while, but the system will not have
	 * correctness guarantees until this is addressed.
	 */
	#latestDateSnapshot: ReadonlyDate;

	// Should always be sorted based on refresh interval, ascending
	#subscriptionTrackers: SubscriptionTracker[] = [];

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

		const dateCopy = Object.freeze(new Date(initialDatetime));
		this.#latestDateSnapshot = dateCopy;
	}

	/**
	 * Convenience method for getting the minimum refresh interval in a
	 * type-safe way
	 */
	#getMinUpdateInterval(): number {
		return (
			this.#subscriptionTrackers[0]?.targetRefreshInterval ??
			Number.POSITIVE_INFINITY
		);
	}

	/**
	 * Attempts to update the current Date snapshot.
	 * @returns {boolean} Indicates whether the state actually changed.
	 */
	#updateSnapshot(): boolean {
		const newSource = new Date(this.#latestDateSnapshot as Date);
		const noStateChange =
			newSource.getMilliseconds() ===
			this.#latestDateSnapshot.getMilliseconds();
		if (noStateChange) {
			return false;
		}

		// Binding the updated state to a separate variable instead of `this`
		// just to make sure the this context can't be messed with as each
		// callback runs
		const frozen = Object.freeze(newSource);
		this.#latestDateSnapshot = frozen;
		return true;
	}

	/**
	 * Updates TimeSync to a new snapshot immediately and synchronously.
	 *
	 * @returns {boolean} Indicates whether there are still subscribers that
	 * need to be notified after the update.
	 */
	#flushSync(): boolean {
		const hasPendingUpdate = this.#updateSnapshot();
		if (hasPendingUpdate && this.#autoNotifyAfterStateUpdate) {
			this.notifySubscribers();
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
		const hasPendingUpdate = this.#updateSnapshot();
		if (hasPendingUpdate) {
			this.notifySubscribers();
		}
	};

	#reconcileTrackersUpdate(): void {
		const oldMin = this.#getMinUpdateInterval();
		this.#subscriptionTrackers.sort(
			(t1, t2) => t2.targetRefreshInterval - t1.targetRefreshInterval,
		);

		const newMin = this.#getMinUpdateInterval();
		if (newMin === oldMin) {
			return;
		}

		if (newMin === Number.POSITIVE_INFINITY) {
			window.clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = undefined;
			return;
		}

		const elapsed = Date.now() - this.#latestDateSnapshot.getMilliseconds();
		const delta = newMin - elapsed;

		// If we're behind on updates, we need to sync immediately before
		// setting up the new interval. With how TimeSync is designed, this case
		// should only be triggered in response to adding a new tracker
		if (delta <= 0) {
			window.clearInterval(this.#latestIntervalId);
			const x = this.#flushSync();
			this.#latestIntervalId = window.setInterval(this.#onTick, newMin);
			return;
		}

		window.clearInterval(this.#latestIntervalId);
		this.#latestIntervalId = window.setInterval(() => {
			window.clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = window.setInterval(this.#onTick, newMin);
		}, delta);
	}

	/**
	 * Adds a new subscription.
	 */
	#addSubscription(targetRefreshInterval: number, onUpdate: OnUpdate): void {
		const prevTracker = this.#subscriptionTrackers.find(
			(t) => t.targetRefreshInterval === targetRefreshInterval,
		);

		let needReconciliation = false;
		let activeTracker: SubscriptionTracker;
		if (prevTracker !== undefined) {
			activeTracker = prevTracker;
		} else {
			needReconciliation = true;
			activeTracker = { targetRefreshInterval, updates: new Map() };
			this.#subscriptionTrackers.push(activeTracker);
		}

		const alreadySubscribed = activeTracker.updates.has(onUpdate);
		if (alreadySubscribed) {
			return;
		}

		const newCount = 1 + (activeTracker.updates.get(onUpdate) ?? 0);
		activeTracker.updates.set(onUpdate, newCount);
		if (needReconciliation) {
			this.#reconcileTrackersUpdate();
		}
	}

	#removeSubscription(targetRefreshInterval: number, onUpdate: OnUpdate): void {
		const activeTracker = this.#subscriptionTrackers.find(
			(t) => t.targetRefreshInterval === targetRefreshInterval,
		);
		if (activeTracker === undefined || !activeTracker.updates.has(onUpdate)) {
			return;
		}

		const newCount = (activeTracker.updates.get(onUpdate) ?? 0) - 1;
		if (newCount > 0) {
			activeTracker.updates.set(onUpdate, newCount);
			return;
		}

		activeTracker.updates.delete(onUpdate);
		// Could probably make it so that we get really specific with which
		// tracker to remove to improve performance, but this setup is easier,
		// and it also gives us a safety net in case we accidentally set up
		// trackers without any functions elsewhere
		const filtered = this.#subscriptionTrackers.filter(
			(t) => t.updates.size > 0,
		);
		if (filtered.length === this.#subscriptionTrackers.length) {
			return;
		}
		this.#subscriptionTrackers = filtered;
		this.#reconcileTrackersUpdate();
	}

	subscribe(hs: SubscriptionHandshake): SubscriptionResult {
		// Destructuring properties so that they can't be fiddled with after
		// this function call ends
		const { targetRefreshInterval, onUpdate } = hs;

		const isInputInvalid =
			!Number.isInteger(targetRefreshInterval) || targetRefreshInterval <= 0;
		if (isInputInvalid) {
			throw new RangeError(
				`TimeSync refresh interval must be a positive integer (received ${targetRefreshInterval}ms)`,
			);
		}

		const capped = Math.max(
			this.#minimumRefreshIntervalMs,
			targetRefreshInterval,
		);
		this.#addSubscription(capped, onUpdate);

		return {
			hasPendingSubscribers: false,
			unsubscribe: () => {
				this.#removeSubscription(capped, onUpdate);
			},
		};
	}

	getStateSnapshot(): ReadonlyDate {
		return this.#latestDateSnapshot;
	}

	notifySubscribers(): void {
		const subscriptionsPaused =
			this.#subscriptionTrackers.length === 0 ||
			this.#getMinUpdateInterval() === Number.POSITIVE_INFINITY;
		if (subscriptionsPaused) {
			return;
		}

		// Copying the latest state into a separate variable, just to make
		// absolutely sure that if the `this` context magically changes between
		// callback calls, it doesn't cause subscribers to receive different
		// values.
		const bound = this.#latestDateSnapshot;
		for (const subEntry of this.#subscriptionTrackers) {
			for (const onUpdate of subEntry.updates.keys()) {
				onUpdate(bound);
			}
		}
	}
}
