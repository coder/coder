/**
 * @todo Things that still need to be done before this can be called done:
 * 1. Explore an idea for handling selectors without forcing someone to specify
 *    data dependencies for what values are accessed via closure:
 *    - A selector is ALWAYS run synchronously in a render. There is no way to
 *      opt out of this behavior, not even memoization
 *    - The selector is still used to determine whether a component should
 *      re-render via a time update. It will be assumed that whatever function
 *      is currently available will always be the most up-to-date
 *    - This approach might make it more viable to combine useTimeSync and
 *      useTimeSyncSelector again
 * 2. Add tests and address any bugs
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
import {
	type SubscriptionEntry,
	TimeSync,
	type TimeSyncInitOptions,
	defaultOptions,
} from "utils/TimeSync";
import { useEffectEvent } from "./hookPolyfills";

export {
	TARGET_REFRESH_ONE_DAY,
	TARGET_REFRESH_ONE_HOUR,
	TARGET_REFRESH_ONE_MINUTE,
	TARGET_REFRESH_ONE_SECOND,
} from "utils/TimeSync";

type ReactSubscriptionCallback = (notifyReact: () => void) => () => void;

type SelectCallback = (newSnapshot: Date) => unknown;

type ReactSubscriptionEntry = Readonly<
	SubscriptionEntry & {
		select?: SelectCallback;
	}
>;

// Need to wrap each value that we put in the selection cache, so that when we
// try to retrieve a value, it's easy to differentiate between a value being
// undefined because that's an explicit selection value, versus it being
// undefined because we forgot to set it in the cache
type SelectionCacheEntry = Readonly<{ value: unknown }>;

interface ReactTimeSyncApi {
	subscribe: (entry: ReactSubscriptionEntry) => () => void;
	getTimeSnapshot: () => Date;
	getSelectionSnapshot: <T = unknown>(id: string) => T;
	invalidateSelection: (id: string, select?: SelectCallback) => void;
}

class ReactTimeSync implements ReactTimeSyncApi {
	readonly #timeSync: TimeSync;
	readonly #selectionCache: Map<string, SelectionCacheEntry>;

	constructor(options: Partial<TimeSyncInitOptions>) {
		this.#timeSync = new TimeSync(options);
		this.#selectionCache = new Map();
	}

	#areValuesDeepEqual(value1: unknown, value2: unknown): boolean {
		// JavaScript is fun and doesn't have a 100% foolproof comparison
		// operation. Object.is covers the most cases, but you still need to
		// compare 0 values, because even though JS programmers almost never
		// care about +0 vs -0, Object.is does treat them as not being equal
		if (Object.is(value1, value2)) {
			return true;
		}
		if (value1 === 0 && value2 === 0) {
			return true;
		}

		if (value1 instanceof Date && value2 instanceof Date) {
			return value1.getMilliseconds() === value2.getMilliseconds();
		}

		// Can't reliably compare functions; just have to treat them as always
		// different. Hopefully no one is storing functions in state for this
		// hook, though
		if (typeof value1 === "function" || typeof value2 === "function") {
			return false;
		}

		if (Array.isArray(value1)) {
			if (!Array.isArray(value2)) {
				return false;
			}
			if (value1.length !== value2.length) {
				return false;
			}
			return value1.every((el, i) => this.#areValuesDeepEqual(el, value2[i]));
		}

		const obj1 = value1 as Record<string, unknown>;
		const obj2 = value1 as Record<string, unknown>;
		if (Object.keys(obj1).length !== Object.keys(obj2).length) {
			return false;
		}
		for (const key in obj1) {
			if (!this.#areValuesDeepEqual(obj1[key], obj2[key])) {
				return false;
			}
		}

		return true;
	}

	// All functions that are part of the public interface must be defined as
	// arrow functions, so they can be passed around React without losing their
	// `this` context

	getTimeSnapshot = () => {
		return this.#timeSync.getTimeSnapshot();
	};

	subscribe = (entry: ReactSubscriptionEntry): (() => void) => {
		const { select, id, targetRefreshInterval, onUpdate } = entry;

		// Make sure that we subscribe first, in case TimeSync is configured to
		// invalidate the snapshot on a new subscription. Want to remove risk of
		// stale data
		const patchedEntry: SubscriptionEntry = {
			id,
			targetRefreshInterval,
			onUpdate: (newDate) => {
				const prevSelection = this.getSelectionSnapshot(id);
				const newSelection = select?.(newDate) ?? newDate;
				if (newSelection === prevSelection) {
					return;
				}

				this.#selectionCache.set(id, { value: newSelection });
				onUpdate(newDate);
			},
		};
		this.#timeSync.subscribe(patchedEntry);

		const date = this.#timeSync.getTimeSnapshot();
		const cacheValue = select?.(date) ?? date;
		this.#selectionCache.set(id, { value: cacheValue });

		return () => this.#timeSync.unsubscribe(id);
	};

	/**
	 * Allows you to grab the result of a selection that has been registered
	 * with ReactTimeSync.
	 *
	 * If this method is called with an ID before a subscription has been
	 * registered for that ID, that will cause the method to throw.
	 */
	getSelectionSnapshot = <T,>(id: string): T => {
		const cacheEntry = this.#selectionCache.get(id);
		if (cacheEntry === undefined) {
			throw new Error(
				"Trying to retrieve value from selection cache without it being initialized",
			);
		}

		return cacheEntry.value as T;
	};

	invalidateSelection = (id: string, select?: SelectCallback): void => {
		const prevSelection = this.#selectionCache.get(id);
		if (prevSelection === undefined) {
			return;
		}

		const dateSnapshot = this.#timeSync.getTimeSnapshot();
		const newSelection = select?.(dateSnapshot) ?? dateSnapshot;
		if (!this.#areValuesDeepEqual(newSelection, prevSelection.value)) {
			this.#selectionCache.set(id, { value: newSelection });
		}
	};
}

const timeSyncContext = createContext<ReactTimeSync | null>(null);

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
		() => new ReactTimeSync(options ?? defaultOptions),
	);

	return (
		<timeSyncContext.Provider value={readonlySync}>
			{children}
		</timeSyncContext.Provider>
	);
};

function useTimeSyncContext(): ReactTimeSync {
	const timeSync = useContext(timeSyncContext);
	if (timeSync === null) {
		throw new Error("Cannot call useTimeSync outside of a TimeSyncProvider");
	}
	return timeSync;
}

type UseTimeSyncOptions = Readonly<{
	/**
	 * targetRefreshInterval is the ideal interval of time, in milliseconds,
	 * that defines how often the hook should refresh with the newest Date
	 * value from TimeSync.
	 *
	 * Note that a refresh is not the same as a re-render. If the hook is
	 * refreshed with a new datetime, but its select callback produces the same
	 * value as before, the hook will skip re-rendering.
	 *
	 * The hook reserves the right to refresh MORE frequently than the
	 * specified value if it would guarantee that the hook does not get out of
	 * sync with other useTimeSync users that are currently mounted on screen.
	 */
	targetRefreshInterval: number;
}>;

export function useTimeSync(options: UseTimeSyncOptions): Date {
	const { targetRefreshInterval } = options;
	const hookId = useId();
	const timeSync = useTimeSyncContext();

	const subscribe = useCallback<ReactSubscriptionCallback>(
		(notifyReact) => {
			return timeSync.subscribe({
				targetRefreshInterval,
				id: hookId,
				onUpdate: notifyReact,
			});
		},
		[hookId, timeSync, targetRefreshInterval],
	);

	const snapshot = useSyncExternalStore(subscribe, timeSync.getTimeSnapshot);
	return snapshot;
}

type UseTimeSyncSelectOptions<T> = Readonly<
	UseTimeSyncOptions & {
		/**
		 * Allows you to transform any date values received from the TimeSync
		 * class. Select functions work similarly to the selects from React
		 * Query and Redux Toolkit – you don't need to memoize them, and they
		 * will only run when the underlying TimeSync state has changed.
		 *
		 * Select functions must not be async. The hook will error out at the
		 * type level if you provide one by mistake.
		 */
		select: (latestDatetime: Date) => T extends Promise<unknown> ? never : T;
	}
>;

/**
 * useTimeSync provides React bindings for the TimeSync class, letting a React
 * component bind its update life cycles to interval updates from TimeSync. This
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
export function useTimeSyncSelect<T>(options: UseTimeSyncSelectOptions<T>): T {
	const { select, targetRefreshInterval } = options;
	const hookId = useId();
	const timeSync = useTimeSyncContext();

	// This is the one place where we're borderline breaking the React rules.
	// useEffectEvent is meant to be used only in useEffect calls, and normally
	// shouldn't be called inside a render. But its behavior lines up with
	// useSyncExternalStore, letting us cheat a little. useSyncExternalStore's
	// state getter callback is called in two scenarios:
	// (1) Mid-render on mount
	// (2) Whenever React is notified of a state change (outside of React).
	//
	// Case 2 is basically an effect with extra steps (and single-threaded JS
	// gives us assurance about correctness). And for (1), useEffectEvent will
	// be initialized with whatever callback you give it on mount. So for the
	// mounting render alone, it's safe to call a useEffectEvent callback from
	// inside a render.
	const externalSelect = useEffectEvent((date: Date): T => {
		const recast = date as Date & T;
		return select?.(recast) ?? recast;
	});

	// We need to define this callback using useCallback instead of
	// useEffectEvent because we want the memoization to be invalidated when the
	// refresh interval changes. (When the subscription callback changes by
	// reference, that causes useSyncExternalStore to redo the subscription with
	// the new callback). All other values need to be included in the dependency
	// array for correctness, but they should always maintain stable memory
	// addresses
	const subscribe = useCallback<ReactSubscriptionCallback>(
		(notifyReact) => {
			return timeSync.subscribe({
				targetRefreshInterval,
				id: hookId,
				onUpdate: notifyReact,
				select: externalSelect,
			});
		},
		[timeSync, hookId, externalSelect, targetRefreshInterval],
	);

	const selection = useSyncExternalStore<T>(subscribe, () => {
		// Need to make sure that we use the un-memoized version of select
		// here because we need to call select callback mid-render to
		// guarantee no stale data. The memoized version only syncs AFTER
		// the current render has finished in full.
		timeSync.invalidateSelection(hookId, select);
		return timeSync.getSelectionSnapshot(hookId);
	});

	return selection;
}
