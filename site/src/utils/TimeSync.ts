type CreateNewDatetime = (prevReadonlyDate: Date) => Date;

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
export type OnUpdate = (newReadonlyDate: Date) => void;

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
	getTimeSnapshot: () => Date;
}

type SubscriptionTracker = Readonly<{
	targetRefreshInterval: number;
	// Each key is an individual onUpdate callback, while each value is the
	// number of subscribers that have registered that callback
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
	#latestDateSnapshot: Date;

	// Should always be sorted based on refresh interval, ascending
	#subscriptions: SubscriptionTracker[] = [];

	// This class uses setInterval for both its intended purpose and as a janky
	// version of setTimeout. There are a few times when we need timeout-like
	// logic, but if we use setInterval for everything, we have fewer overall
	// data dependencies and don't have to juggle different IDs
	#latestIntervalId: number | undefined = undefined;

	constructor(options: Partial<TimeSyncInitOptions>) {
		const {
			initialDatetime = defaultOptions.initialDatetime,
			resyncOnNewSubscription = defaultOptions.resyncOnNewSubscription,
			createNewDatetime = defaultOptions.createNewDatetime,
		} = options;

		this.#createNewDatetime = createNewDatetime;
		this.#resyncOnNewSubscription = resyncOnNewSubscription;

		const dateCopy = new Date(initialDatetime);
		this.#latestDateSnapshot = dateCopy;
	}

	// Convenience method for getting the minimum refresh interval in a
	// type-safe way
	#getMinUpdateInterval(): number {
		return (
			this.#subscriptions[0]?.targetRefreshInterval ?? Number.POSITIVE_INFINITY
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
			this.#subscriptions.length === 0 ||
			this.#getMinUpdateInterval() === Number.POSITIVE_INFINITY;
		if (subscriptionsPaused) {
			return;
		}
		for (const subEntry of this.#subscriptions) {
			for (const onUpdate of subEntry.updates.keys()) {
				onUpdate(frozen);
			}
		}
	};

	#reconcileAdd(): void {}

	#reconcileRemoval(): void {
		const filtered = this.#subscriptions.filter((t) => t.updates.size > 0);
		if (filtered.length === this.#subscriptions.length) {
			return;
		}

		const oldMin = this.#getMinUpdateInterval();
		filtered.sort(
			(t1, t2) => t2.targetRefreshInterval - t1.targetRefreshInterval,
		);
		this.#subscriptions = filtered;

		const newMin = this.#getMinUpdateInterval();
		if (newMin === oldMin) {
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
		const prevTracker = this.#subscriptions.find(
			(t) => t.targetRefreshInterval === targetRefreshInterval,
		);

		// Using try/finally as a janky version of Go's defer keyword. Need to
		// make sure reconciliation triggers when needed, regardless of which
		// code path we take
		let needReconciliation = false;
		try {
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

			const newCount = activeTracker.updates.get(onUpdate) ?? 0;
			activeTracker.updates.set(onUpdate, newCount);
			if (!alreadySubscribed && this.#resyncOnNewSubscription) {
				this.#updateDateSnapshot();
			}
		} finally {
			if (needReconciliation) {
				this.#reconcileAdd();
			}
		}
	}

	#removeSubscription(onUpdate: OnUpdate): void {
		let needReconciliation = false;
		for (const tracker of this.#subscriptions) {
			const cbIndex = tracker.updates.indexOf(onUpdate);
			if (cbIndex === -1) {
				continue;
			}
			tracker.updates.splice(cbIndex, 1);
			needReconciliation ||= tracker.updates.length === 0;
		}
		if (needReconciliation) {
			this.#reconcileRemoval();
		}
	}

	subscribe(hs: SubscriptionHandshake): () => void {
		const { targetRefreshInterval, onUpdate } = hs;
		const isInputInvalid =
			!Number.isInteger(targetRefreshInterval) || targetRefreshInterval <= 0;
		if (isInputInvalid) {
			throw new RangeError(
				`TimeSync refresh interval must be a positive integer (received ${targetRefreshInterval}ms)`,
			);
		}

		this.#addSubscription(targetRefreshInterval, onUpdate);
		return () => this.#removeSubscription(onUpdate);
	}

	/**
	 * @todo While this isn't likely, in order to make sure that the system
	 * stays airtight and can't break down, we need to make sure that the Date
	 * returned forbids being mutated. Tried rolling it out for the initial
	 * version, but figuring out the internal ergonomics while building out
	 * everything else got to be a bit much.
	 *
	 * Using Object.freeze isn't good enough, because it doesn't stop the
	 * built-in set methods from modifying the internal state. We can probably
	 * get away with a Proxy object, and set it up to turn properties prefixed
	 * with `set` into a no-op
	 */
	getTimeSnapshot(): Date {
		return this.#latestDateSnapshot;
	}
}
