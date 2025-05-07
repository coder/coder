/**
 * @todo Things that still need to be done before this can be called done:
 *
 * 1. Fill out all incomplete methods
 * 2. Add tests
 * 3. See if there's a way to make sure that if you provide a type parameter to
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
import {
	type SubscriptionEntry,
	TimeSync,
	type TimeSyncInitOptions,
	defaultOptions,
} from "utils/TimeSync";
import { useEffectEvent } from "./hookPolyfills";

export {
	IDEAL_REFRESH_ONE_DAY,
	IDEAL_REFRESH_ONE_HOUR,
	IDEAL_REFRESH_ONE_MINUTE,
	IDEAL_REFRESH_ONE_SECOND,
} from "utils/TimeSync";

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

	// All functions that are part of the public interface must be defined as
	// arrow functions, so that they work properly with React

	subscribe = (entry: ReactSubscriptionEntry): (() => void) => {
		const { select, id, idealRefreshIntervalMs, onUpdate } = entry;

		// Make sure that we subscribe first, in case TimeSync is configured to
		// invalidate the snapshot on a new subscription. Want to remove risk of
		// stale data
		const patchedEntry: SubscriptionEntry = {
			id,
			idealRefreshIntervalMs,
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
		const cacheEntry = this.#selectionCache.get(id);
		if (cacheEntry === undefined) {
			return;
		}

		const dateSnapshot = this.#timeSync.getTimeSnapshot();
		const newSelection = select?.(dateSnapshot) ?? dateSnapshot;
		this.#selectionCache.set(id, { value: newSelection });
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

type UseTimeSyncOptions<T = Date> = Readonly<{
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

	/**
	 * selectDependencies acts like the dependency array for a useMemo callback.
	 * Whenever any of the elements in the array change by value, that will
	 * cause the select callback to re-run synchronously and produce a new,
	 * up-to-date value for the current render.
	 */
	selectDependencies?: readonly unknown[];

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
export function useTimeSync<T = Date>(options: UseTimeSyncOptions<T>): T {
	const {
		select,
		selectDependencies: selectDeps,
		targetRefreshInterval: idealRefreshIntervalMs,
	} = options;
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
	// gives us assurance about correctness). And for (1), useEffectEvent will
	// be initialized with whatever callback you give it on mount. So for the
	// mounting render alone, it's safe to call a useEffectEvent callback from
	// inside a render.
	const selectForOutsideReact = useEffectEvent((date: Date): T => {
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
	type ReactSubscriptionCallback = (notifyReact: () => void) => () => void;
	const subscribe = useCallback<ReactSubscriptionCallback>(
		(notifyReact) => {
			return timeSync.subscribe({
				idealRefreshIntervalMs,
				id: hookId,
				onUpdate: notifyReact,
				select: selectForOutsideReact,
			});
		},
		[timeSync, hookId, selectForOutsideReact, idealRefreshIntervalMs],
	);

	const [prevDeps, setPrevDeps] = useState(selectDeps);
	const depsAreInvalidated = areDepsInvalidated(prevDeps, selectDeps);

	const selection = useSyncExternalStore<T>(subscribe, () => {
		if (depsAreInvalidated) {
			// Need to make sure that we use the un-memoized version of select
			// here because we need to call select callback mid-render to
			// guarantee no stale data. The memoized version only syncs AFTER
			// the current render has finished in full.
			timeSync.invalidateSelection(hookId, select);
		}
		return timeSync.getSelectionSnapshot(hookId);
	});

	// Setting state mid-render like this is valid, but we just need to make
	// sure that we wait until after the useSyncExternalStore state getter runs
	if (depsAreInvalidated) {
		setPrevDeps(selectDeps);
	}

	return selection;
}

function areDepsInvalidated(
	oldDeps: readonly unknown[] | undefined,
	newDeps: readonly unknown[] | undefined,
): boolean {
	if (oldDeps === undefined) {
		if (newDeps === undefined) {
			return false;
		}
		return true;
	}

	const oldRecast = oldDeps as readonly unknown[];
	const newRecast = oldDeps as readonly unknown[];
	if (oldRecast.length !== newRecast.length) {
		return true;
	}

	for (const [index, el] of oldRecast.entries()) {
		if (el !== newRecast[index]) {
			return true;
		}
	}

	return false;
}
