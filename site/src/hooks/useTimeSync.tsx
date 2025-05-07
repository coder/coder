/**
 * @todo Things that still need to be done before this can be called done:
 *
 * 1. Revamp the entire class definition, and fill out all missing methods
 * 2. Update the class to respect the resyncOnNewSubscription option
 * 3. Add tests
 * 4. See if there's a way to make sure that if you provide a type parameter to
 *    the hook, you must also provide a select function
 */
import {
	type FC,
	type PropsWithChildren,
	createContext,
	useCallback,
	useContext,
	useId,
	useState,
	useSyncExternalStore,
} from "react";
import { useEffectEvent } from "./hookPolyfills";

export const IDEAL_REFRESH_ONE_SECOND = 1_000;
export const IDEAL_REFRESH_ONE_MINUTE = 60 * 1_000;
export const IDEAL_REFRESH_ONE_HOUR = 60 * 60 * 1_000;
export const IDEAL_REFRESH_ONE_DAY = 24 * 60 * 60 * 1_000;

type SetInterval = (fn: () => void, intervalMs: number) => number;
type ClearInterval = (id: number | undefined) => void;

type TimeSyncInitOptions = Readonly<{
	/**
	 * Configures whether adding a new subscription will immediately create a
	 * new time snapshot and use it to update all other subscriptions.
	 */
	resyncOnNewSubscription: boolean;

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
}>;

const defaultOptions: TimeSyncInitOptions = {
	initialDatetime: new Date(),
	resyncOnNewSubscription: true,
	createNewDatetime: () => new Date(),
	setInterval: window.setInterval,
	clearInterval: window.clearInterval,
};

type SubscriptionEntry = Readonly<{
	id: string;
	idealRefreshIntervalMs: number;
	onUpdate: (newDatetime: Date) => void;
	select?: (newSnapshot: Date) => unknown;
}>;

interface TimeSyncApi {
	subscribe: (entry: SubscriptionEntry) => () => void;
	unsubscribe: (id: string) => void;
	getTimeSnapshot: () => Date;
	getSelectionSnapshot: <T = unknown>(id: string) => T;
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
	readonly #selectionCache: Map<string, unknown>;

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
		this.#selectionCache = new Map();
		this.#latestIntervalId = undefined;
	}

	#reconcileRefreshIntervals(): void {
		if (this.#subscriptions.length === 0) {
			this.#clearInterval(this.#latestIntervalId);
			return;
		}

		const prevFastestInterval =
			this.#subscriptions[0]?.idealRefreshIntervalMs ??
			Number.POSITIVE_INFINITY;
		if (this.#subscriptions.length > 1) {
			this.#subscriptions.sort(
				(e1, e2) => e1.idealRefreshIntervalMs - e2.idealRefreshIntervalMs,
			);
		}

		const newFastestInterval =
			this.#subscriptions[0]?.idealRefreshIntervalMs ??
			Number.POSITIVE_INFINITY;
		if (prevFastestInterval === newFastestInterval) {
			return;
		}
		if (newFastestInterval === Number.POSITIVE_INFINITY) {
			this.#clearInterval(this.#latestIntervalId);
			return;
		}

		/**
		 * @todo Figure out the conditions when the interval should be set up, and
		 * when/how it should be updated
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
			if (subEntry.select === undefined) {
				subEntry.onUpdate(this.#latestDateSnapshot);
				continue;
			}

			// Keeping things simple by only comparing values React-style with ===.
			// If that becomes a problem down the line, we can beef the class up
			const prevSelection = this.#selectionCache.get(subEntry.id);
			const newSelection = subEntry.select(this.#latestDateSnapshot);
			if (prevSelection !== newSelection) {
				this.#selectionCache.set(subEntry.id, newSelection);
				subEntry.onUpdate(this.#latestDateSnapshot);
			}
		}
	}

	// All functions that are part of the public interface must be defined as
	// arrow functions, so that they work properly with React

	getTimeSnapshot = (): Date => {
		return this.#latestDateSnapshot;
	};

	getSelectionSnapshot = <T,>(id: string): T => {
		return this.#selectionCache.get(id) as T;
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
		if (entry.idealRefreshIntervalMs <= 0) {
			throw new Error(
				`Refresh interval ${entry.idealRefreshIntervalMs} must be a positive integer (or Infinity)`,
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
		if (prev.idealRefreshIntervalMs !== entry.idealRefreshIntervalMs) {
			this.#reconcileRefreshIntervals();
		}
		return unsub;
	};
}

const timeSyncContext = createContext<TimeSync | null>(null);

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
	// Making the TimeSync instance be initialized via React State, so that it's
	// easy to mock it out for component and story tests. TimeSync itself should
	// be treated like a pseudo-ref value, where its values can only be used in
	// very specific, React-approved ways
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
	idealRefreshIntervalMs: number;

	/**
	 * Allows you to transform any date values received from the TimeSync class.
	 * Select functions work similarly to the selects from React Query and Redux
	 * Toolkit – you don't need to memoize them, and they will only run when the
	 * underlying TimeSync state has changed.
	 *
	 * Select functions must not be async. The hook will error out at the type
	 * level if you provide one by mistake.
	 */
	select?: (latestDatetime: Date) => T extends Promise<unknown> ? never : T;
}>;

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
	const { select, idealRefreshIntervalMs } = options;
	const timeSync = useContext(timeSyncContext);
	if (timeSync === null) {
		throw new Error("Cannot call useTimeSync outside of a TimeSyncProvider");
	}

	// Abusing useId a little bit here. It's mainly meant to be used for
	// accessibility, but it also gives us a globally unique ID associated with
	// whichever component instance is consuming this hook
	const hookId = useId();

	// This is the one place where we're borderline breaking the React rules.
	// useEffectEvent is meant to be used only in useEffect calls, and normally
	// shouldn't be called inside a render. But its behavior lines up with
	// useSyncExternalStore, letting us cheat a little. useSyncExternalStore's
	// state getter callback is called in two scenarios:
	// (1) Mid-render on mount
	// (2) Whenever React is notified of a state change (outside of React).
	//
	// Case 2 is basically an effect with extra steps (and single-threaded JS
	// gives us assurance about correctness). And for (1), useEffectEvent will be
	// initialized with whatever callback you give it on mount. So for the
	// mounting render alone, it's safe to call a useEffectEvent callback from
	// inside a render.
	const stableSelect = useEffectEvent((date: Date): T => {
		const recast = date as Date & T;
		return select?.(recast) ?? recast;
	});

	// We need to define this callback using useCallback instead of
	// useEffectEvent because we want the memoization to be invaliated when the
	// refresh interval changes. (When the subscription callback changes by
	// reference, that causes useSyncExternalStore to redo the subscription with
	// the new callback). All other values need to be included in the dependency
	// array for correctness, but they should always maintain stable memory
	// addresses
	type ReactSubscriptionCallback = (notifyReact: () => void) => () => void;
	const subscribe = useCallback<ReactSubscriptionCallback>(
		(notifyReact) => {
			return timeSync.subscribe({
				idealRefreshIntervalMs,
				id: hookId,
				onUpdate: notifyReact,
				select: stableSelect,
			});
		},
		[timeSync, hookId, stableSelect, idealRefreshIntervalMs],
	);

	const snapshot = useSyncExternalStore<T>(subscribe, () =>
		timeSync.getSelectionSnapshot(hookId),
	);

	return snapshot;
}
