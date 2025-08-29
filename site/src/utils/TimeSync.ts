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

/**
 * The callback to call when a new state update has been created. If the
 * callback is already registered with TimeSync for the specified refresh
 * interval, that becomes a no-op.
 *
 * If the callback is already registered, but for a different refresh
 * interval, that causes the update interval to get updated internally. It
 * does not register the same callback multiple times.
 */
export type OnUpdate = (newDatetime: Date) => void;

export type SubscriptionEntry = Readonly<{
	/**
	 * The maximum update interval that a subscriber needs. TimeSync will always
	 * use the *lowest* interval amongst all subscribers to update everything.
	 *
	 * If all subscribers have a value of Number.POSITIVE_INFINITY, no updates
	 * will ever be dispatched.
	 */
	targetRefreshInterval: number;
	onUpdate: OnUpdate;
}>;

const defaultOptions: TimeSyncInitOptions = {
	initialDatetime: new Date(),
	resyncOnNewSubscription: true,
	createNewDatetime: () => new Date(),
	setInterval: window.setInterval,
	clearInterval: window.clearInterval,
};

interface TimeSyncApi {
	/**
	 * Subscribes an external system to TimeSync. Call the returned callback to
	 * unsubscribe the system.
	 *
	 * @throws {RangeError} If the provided interval is less than or equal to 0.
	 */
	subscribe: (entry: SubscriptionEntry) => () => void;

	/**
	 * Allows any system to pull the newest time state from TimeSync, regardless
	 * of whether the system is subscribed
	 *
	 * The state returned out is a direct reference to the latest time state,
	 * but the value is frozen at runtime to prevent accidental mutations.
	 */
	getTimeSnapshot: () => Date;
}

type SubscriptionTracker = {
	readonly targetRefreshInterval: number;
	updates: OnUpdate[];
};

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
	readonly #createNewDatetime: (previousDate: Date) => Date;
	readonly #setInterval: SetInterval;
	readonly #clearInterval: ClearInterval;

	// Subscriptions should always be sorted with the smallest update intervals
	// being listed first
	#subscriptions: SubscriptionTracker[] = [];
	#latestIntervalId: number | undefined = undefined;
	#latestDateSnapshot: Date;

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

		const dateCopy = Object.freeze(new Date(initialDatetime));
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
		const newRawDate = this.#createNewDatetime(this.#latestDateSnapshot);
		const noStateChange =
			newRawDate.getMilliseconds() ===
			this.#latestDateSnapshot.getMilliseconds();
		if (noStateChange) {
			return;
		}

		// Binding the updated state to a separate variable instead of `this`
		// just to make sure the this context can't be messed with as each
		// callback runs
		const newFrozenDate = Object.freeze(newRawDate);
		this.#latestDateSnapshot = newFrozenDate;

		const subscriptionsPaused =
			this.#subscriptions.length === 0 ||
			this.#getMinUpdateInterval() === Number.POSITIVE_INFINITY;
		if (subscriptionsPaused) {
			return;
		}
		for (const subEntry of this.#subscriptions) {
			for (const onUpdate of subEntry.updates) {
				onUpdate(newFrozenDate);
			}
		}
	};

	#reconcileAdd(): void {}

	#reconcileRemoval(): void {
		const filtered = this.#subscriptions.filter((t) => t.updates.length > 0);
		if (filtered.length === this.#subscriptions.length) {
			return;
		}

		const oldMin = this.#getMinUpdateInterval();
		this.#subscriptions = filtered;
		this.#subscriptions.sort(
			(t1, t2) => t2.targetRefreshInterval - t1.targetRefreshInterval,
		);

		const newMin = this.#getMinUpdateInterval();
		if (newMin === oldMin) {
			return;
		}

		const elapsed = Date.now() - this.#latestDateSnapshot.getMilliseconds();
		const delta = newMin - elapsed;

		// If we're behind on updates, we need to sync immediately before
		// setting up the new interval
		if (delta <= 0) {
			this.#clearInterval(this.#latestIntervalId);
			this.#updateDateSnapshot();
			this.#latestIntervalId = this.#setInterval(
				this.#updateDateSnapshot,
				newMin,
			);
			return;
		}

		// Otherwise, we'll use setInterval as a janky version of setTimeout so
		// that we we have fewer overall data dependencies
		this.#clearInterval(this.#latestIntervalId);
		const pseudoTimeout = () => {
			this.#clearInterval(this.#latestIntervalId);
			this.#latestIntervalId = this.#setInterval(
				this.#updateDateSnapshot,
				newMin,
			);
		};
		this.#latestIntervalId = this.#setInterval(pseudoTimeout, delta);
	}

	#addSubscription(targetRefreshInterval: number, onUpdate: OnUpdate): void {
		const prevTracker = this.#subscriptions.find(
			(t) => t.targetRefreshInterval === targetRefreshInterval,
		);

		let needReconciliation = false;
		try {
			let activeTracker: SubscriptionTracker;
			if (prevTracker !== undefined) {
				activeTracker = prevTracker;
			} else {
				needReconciliation = true;
				activeTracker = { targetRefreshInterval, updates: [] };
			}

			const alreadySubscribed =
				activeTracker.updates.includes(onUpdate) ?? false;
			if (alreadySubscribed) {
				return;
			}

			// Handle de-duplicating callback from other trackers
			for (const tracker of this.#subscriptions) {
				if (tracker === activeTracker) {
					tracker.updates.push(onUpdate);
					continue;
				}

				const prevIndex = tracker.updates.indexOf(onUpdate);
				if (prevIndex === -1) {
					continue;
				}

				tracker.updates.splice(prevIndex, 1);
				needReconciliation ||= tracker.updates.length === 0;
			}
		} finally {
			if (needReconciliation) {
				this.#reconcileAdd();
			}
			if (prevTracker === undefined && this.#resyncOnNewSubscription) {
				this.#updateDateSnapshot();
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

	subscribe(entry: SubscriptionEntry): () => void {
		const { targetRefreshInterval, onUpdate } = entry;
		const isInputInvalid =
			!Number.isInteger(targetRefreshInterval) || targetRefreshInterval <= 0;
		if (isInputInvalid) {
			throw new RangeError(
				`Provided refresh interval ${targetRefreshInterval} must be a positive integer`,
			);
		}

		this.#addSubscription(targetRefreshInterval, onUpdate);
		return () => this.#removeSubscription(onUpdate);
	}

	getTimeSnapshot(): Date {
		return this.#latestDateSnapshot;
	}
}
