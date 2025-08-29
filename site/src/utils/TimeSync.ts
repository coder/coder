/**
 * A readonly version of a Date object. The object lacks all set methods at the
 * type level, and is frozen at runtime.
 */
export type ReadonlyDate = Omit<Date, `set${string}`>;

export type CreateNewDatetime = (prevDate: ReadonlyDate) => Date;

export type TimeSyncInitOptions = Readonly<{
	/**
	 * The Date value to use when initializing a TimeSync instance.
	 */
	initialDatetime: Date;

	/**
	 * The function to use when creating a new datetime snapshot when a TimeSync
	 * needs to update on an interval.
	 */
	createNewDatetime: CreateNewDatetime;

	/**
	 * Configures whether adding a new subscription will immediately create
	 * a new time snapshot and use it to update all other subscriptions.
	 * Defaults to true.
	 */
	resyncOnNewSubscription: boolean;
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
	resyncOnNewSubscription: true,
	createNewDatetime: () => new Date(),
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
	 * Returns a callback for explicitly unsubscribing. Unsubscribing only
	 * removes a single subscription instance. If other subscribers have
	 * registered the exact same callback, they will not be affected.
	 *
	 * @throws {RangeError} If the provided interval is less than or equal to 0.
	 */
	subscribe: (entry: SubscriptionHandshake) => () => void;

	/**
	 * Allows any system to pull the newest time state from TimeSync, regardless
	 * of whether the system is subscribed
	 */
	getTimeSnapshot: () => ReadonlyDate;
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
	readonly #resyncOnNewSubscription: boolean;
	readonly #createNewDatetime: CreateNewDatetime;

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

	constructor(options: Partial<TimeSyncInitOptions>) {
		const {
			initialDatetime = defaultOptions.initialDatetime,
			resyncOnNewSubscription = defaultOptions.resyncOnNewSubscription,
			createNewDatetime = defaultOptions.createNewDatetime,
		} = options;

		this.#createNewDatetime = createNewDatetime;
		this.#resyncOnNewSubscription = resyncOnNewSubscription;

		const dateCopy = Object.freeze(new Date(initialDatetime));
		this.#latestDateSnapshot = dateCopy;
	}

	// Convenience method for getting the minimum refresh interval in a
	// type-safe way
	#getMinUpdateInterval(): number {
		return (
			this.#subscriptionTrackers[0]?.targetRefreshInterval ??
			Number.POSITIVE_INFINITY
		);
	}

	// Defined as an arrow function so that we can just pass it directly to
	// setInterval without needing to make a new wrapper function each time. We
	// don't have many situations where we can lose the `this` context, but this
	// is one of them
	#updateDateSnapshot = (): void => {
		const newSource = this.#createNewDatetime(this.#latestDateSnapshot);
		const noStateChange =
			newSource.getMilliseconds() ===
			this.#latestDateSnapshot.getMilliseconds();
		if (noStateChange) {
			return;
		}

		// Binding the updated state to a separate variable instead of `this`
		// just to make sure the this context can't be messed with as each
		// callback runs
		const frozen = Object.freeze(newSource);
		this.#latestDateSnapshot = frozen;

		const subscriptionsPaused =
			this.#subscriptionTrackers.length === 0 ||
			this.#getMinUpdateInterval() === Number.POSITIVE_INFINITY;
		if (subscriptionsPaused) {
			return;
		}
		for (const subEntry of this.#subscriptionTrackers) {
			for (const onUpdate of subEntry.updates.keys()) {
				onUpdate(frozen);
			}
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
		// setting up the new interval
		if (delta <= 0) {
			window.clearInterval(this.#latestIntervalId);
			this.#updateDateSnapshot();
			this.#latestIntervalId = window.setInterval(
				this.#updateDateSnapshot,
				newMin,
			);
			return;
		}

		window.clearInterval(this.#latestIntervalId);
		this.#latestIntervalId = window.setInterval(() => {
			window.clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = window.setInterval(
				this.#updateDateSnapshot,
				newMin,
			);
		}, delta);
	}

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
		if (!alreadySubscribed && this.#resyncOnNewSubscription) {
			this.#updateDateSnapshot();
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

	subscribe(hs: SubscriptionHandshake): () => void {
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

		this.#addSubscription(targetRefreshInterval, onUpdate);
		return () => this.#removeSubscription(targetRefreshInterval, onUpdate);
	}

	getTimeSnapshot(): ReadonlyDate {
		return this.#latestDateSnapshot;
	}
}
