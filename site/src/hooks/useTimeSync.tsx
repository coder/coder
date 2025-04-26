import {
	createContext,
	useCallback,
	useContext,
	useId,
	useState,
	useSyncExternalStore,
	type FC,
	type PropsWithChildren,
} from "react";

export const MAX_REFRESH_ONE_SECOND = 1_000;
export const MAX_REFRESH_ONE_MINUTE = 60 * 1_000;
export const MAX_REFRESH_ONE_HOUR = 60 * 60 * 1_000;
export const MAX_REFRESH_ONE_DAY = 24 * 60 * 60 * 1_000;

type SetInterval = (fn: () => void, intervalMs: number) => number;
type ClearInterval = (id: number | undefined) => void;

type TimeSyncInitOptions = Readonly<{
	/**
	 * Indicates whether adding a new subscription will immediately create a new
	 * time snapshot, and update all other subscribers with that snapshot.
	 */
	resyncOnNewSubscription: boolean;
	initialDatetime: Date;
	createNewDateime: (prevDatetime: Date) => Date;
	setInterval: SetInterval;
	clearInterval: ClearInterval;
}>;

const defaultOptions: TimeSyncInitOptions = {
	initialDatetime: new Date(),
	resyncOnNewSubscription: true,
	createNewDateime: () => new Date(),
	setInterval: window.setInterval,
	clearInterval: window.clearInterval,
};

type SubscriptionEntry = Readonly<{
	id: string;
	maxRefreshIntervalMs: number;
	onUpdate: (newDatetime: Date) => void;
}>;

interface TimeSyncApi {
	getLatestDatetimeSnapshot: () => Date;
	subscribe: (entry: SubscriptionEntry) => () => void;
	unsubscribe: (id: string) => void;
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
	readonly resyncOnNewSubscription: boolean;
	readonly #createNewDatetime: (prev: Date) => Date;
	readonly #setInterval: SetInterval;
	readonly #clearInterval: ClearInterval;

	#latestSnapshot: Date;
	#subscriptions: SubscriptionEntry[];
	#latestIntervalId: number | undefined;

	constructor(options: Partial<TimeSyncInitOptions>) {
		const {
			initialDatetime = defaultOptions.initialDatetime,
			resyncOnNewSubscription = defaultOptions.resyncOnNewSubscription,
			createNewDateime = defaultOptions.createNewDateime,
			setInterval = defaultOptions.setInterval,
			clearInterval = defaultOptions.clearInterval,
		} = options;

		this.#latestSnapshot = initialDatetime;
		this.resyncOnNewSubscription = resyncOnNewSubscription;
		this.#subscriptions = [];
		this.#latestIntervalId = undefined;
		this.#setInterval = setInterval;
		this.#clearInterval = clearInterval;
		this.#createNewDatetime = createNewDateime;
	}

	#reconcileRefreshIntervals(): void {
		if (this.#subscriptions.length === 0) {
			this.#clearInterval(this.#latestIntervalId);
			return;
		}

		const prevFastestInterval =
			this.#subscriptions[0]?.maxRefreshIntervalMs ?? Infinity;
		if (this.#subscriptions.length > 1) {
			this.#subscriptions.sort(
				(e1, e2) => e1.maxRefreshIntervalMs - e2.maxRefreshIntervalMs,
			);
		}

		const newFastestInterval =
			this.#subscriptions[0]?.maxRefreshIntervalMs ?? Infinity;
		if (prevFastestInterval === newFastestInterval) {
			return;
		}
		if (newFastestInterval === Infinity) {
			this.#clearInterval(this.#latestIntervalId);
			return;
		}

		/**
		 * @todo Figure out the conditions when the interval should be set up, and
		 * when/how it should be updated
		 */
		this.#latestIntervalId = this.#setInterval(() => {
			this.#latestSnapshot = this.#createNewDatetime(this.#latestSnapshot);
			this.#notifySubscriptions();
		}, newFastestInterval);
	}

	#notifySubscriptions(): void {
		for (const subEntry of this.#subscriptions) {
			subEntry.onUpdate(this.#latestSnapshot);
		}
	}

	// All functions that are part of the public interface must be defined as
	// arrow functions, so that they work properly with React

	getLatestDatetimeSnapshot = (): Date => {
		return this.#latestSnapshot;
	};

	unsubscribe = (id: string): void => {
		const updated = this.#subscriptions.filter((s) => s.id !== id);
		if (updated.length === this.#subscriptions.length) {
			return;
		}

		this.#subscriptions = updated;
		this.#reconcileRefreshIntervals();
	};

	subscribe = (entry: SubscriptionEntry): (() => void) => {
		if (entry.maxRefreshIntervalMs <= 0) {
			throw new Error(
				`Refresh interval ${entry.maxRefreshIntervalMs} must be a positive integer (or Infinity)`,
			);
		}

		const unsub = () => this.unsubscribe(entry.id);
		const subIndex = this.#subscriptions.findIndex((s) => s.id === entry.id);
		if (subIndex === -1) {
			this.#subscriptions.push(entry);
			this.#reconcileRefreshIntervals();
			return unsub;
		}

		const prev = this.#subscriptions[subIndex];
		if (prev === undefined) {
			throw new Error("Went out of bounds");
		}

		this.#subscriptions[subIndex] = entry;
		if (prev.maxRefreshIntervalMs !== entry.maxRefreshIntervalMs) {
			this.#reconcileRefreshIntervals();
		}
		return unsub;
	};
}

const timeSyncContext = createContext<TimeSync | null>(null);

function useTimeSyncContext(): TimeSync {
	const timeSync = useContext(timeSyncContext);
	if (timeSync === null) {
		throw new Error("Cannot call useTimeSync outside of a TimeSyncProvider");
	}

	return timeSync;
}

type TimeSyncProviderProps = Readonly<
	PropsWithChildren<{
		options?: Partial<TimeSyncInitOptions>;
	}>
>;

/**
 * TimeSyncProvider provides an easy way to dependency-inject a TimeSync
 * instance throughout a React application.
 *
 * Note that whatever options are provided on the first render will be locked in
 * for the lifetime of the provider. There is no way to reconfigure options on
 * re-renders.
 */
export const TimeSyncProvider: FC<TimeSyncProviderProps> = ({
	children,
	options,
}) => {
	// Making the TimeSync instance be initalized via React State, so that it's
	// easy to mock it out for component and story tests. TimeSync itself should
	// be treated like a pseudo-ref value, where its values can only be used in
	// very specific, React-approved ways (e.g., not directly in a render path)
	const [readonlySync] = useState(
		() => new TimeSync(options ?? defaultOptions),
	);

	return (
		<timeSyncContext.Provider value={readonlySync}>
			{children}
		</timeSyncContext.Provider>
	);
};

type UseTimeSyncOptions<T = Date> = Readonly<{
	maxRefreshIntervalMs: number;

	/**
	 * Allows you to transform any date values received from the TimeSync class.
	 *
	 * Note that select functions are not memoized and will run on every render
	 * (similar to the ones in React Query and Redux Toolkit's default selectors).
	 * Select functions should be kept cheap to recalculate.
	 *
	 * Select functions must not be async. The hook will error out at the type
	 * level if you provide one by mistake.
	 */
	select?: (newDate: Date) => T extends Promise<unknown> ? never : T;
}>;

type ReactSubscriptionCallback = (notifyReact: () => void) => () => void;

/**
 * useTimeSync provides React bindings for the TimeSync class, letting a React
 * component bind its update lifecycles to interval updates from TimeSync. This
 * hook should be used anytime you would want to use a Date instance directly in
 * a component render path.
 *
 * By default, it returns the raw Date value from the most recent update, but
 * by providing a `select` callback in the options, you can get a transformed
 * version of the time value, using 100% pure functions.
 *
 * By specifying a value of positive Infinity, that indicates that the hook will
 * not update by itself. But if another component is mounted with a more
 * frequent update interval, both component instances will update on that
 * interval.
 */
export function useTimeSync<T = Date>(options: UseTimeSyncOptions<T>): T {
	const { select, maxRefreshIntervalMs } = options;

	// Abusing useId a little bit here. It's mainly meant to be used for
	// accessibility, but it also gives us a globally unique ID associated with
	// whichever component instance is consuming this hook
	const hookId = useId();
	const timeSync = useTimeSyncContext();

	// We need to define this callback using useCallback instead of useEffectEvent
	// because we want the memoization to be invaliated when the refresh interval
	// changes. (When the subscription callback changes by reference, that causes
	// useSyncExternalStore to redo the subscription with the new callback). All
	// other values need to be included in the dependency array for correctness,
	// but their memory references should always be stable
	const subscribe = useCallback<ReactSubscriptionCallback>(
		(notifyReact) => {
			return timeSync.subscribe({
				maxRefreshIntervalMs,
				onUpdate: notifyReact,
				id: hookId,
			});
		},
		[timeSync, hookId, maxRefreshIntervalMs],
	);

	const currentTime = useSyncExternalStore(subscribe, () =>
		timeSync.getLatestDatetimeSnapshot(),
	);

	const recast = currentTime as T & Date;
	return select?.(recast) ?? recast;
}
