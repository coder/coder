/**
 * @todo Things that still need to be done before this can be called done:
 *
 * 1. Fill out all incomplete methods
 * 2. Add tests
 * 3. See if there's a way to make sure that if you provide a type parameter to
 *    the hook, you must also provide a select function
 */
import {
	createContext,
	type FC,
	type PropsWithChildren,
	useCallback,
	useContext,
	useId,
	useState,
	useSyncExternalStore,
} from "react";
import {
	defaultOptions,
	type SubscriptionEntry,
	TimeSync,
	type TimeSyncInitOptions,
} from "utils/TimeSync";
import { useEffectEvent } from "./hookPolyfills";

export {
	IDEAL_REFRESH_ONE_DAY,
	IDEAL_REFRESH_ONE_HOUR,
	IDEAL_REFRESH_ONE_MINUTE,
	IDEAL_REFRESH_ONE_SECOND,
} from "utils/TimeSync";

type ReactSubscriptionCallback = (notifyReact: () => void) => () => void;

type ReactTimeSyncSubscriptionEntry = Readonly<
	SubscriptionEntry & {
		select?: (newSnapshot: Date) => unknown;
	}
>;

// Need to wrap each value that we put in the selection cache, so that when we
// try to retrieve a value, it's easy to differentiate between a value being
// undefined because that's an explicit selection value, versus it being
// undefined because we forgot to set it in the cache
type SelectionCacheEntry = Readonly<{ value: unknown }>;

interface ReactTimeSyncApi {
	subscribe: (entry: ReactTimeSyncSubscriptionEntry) => () => void;
	getSelectionSnapshot: <T = unknown>(id: string) => T;
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

	subscribe = (entry: ReactTimeSyncSubscriptionEntry): (() => void) => {
		const { select, id, idealRefreshIntervalMs, onUpdate } = entry;

		// Make sure that we subscribe first, in case TimeSync is configured to
		// invalidate the snapshot on a new subscription. Want to remove risk of
		// stale data
		const patchedEntry: SubscriptionEntry = {
			id,
			idealRefreshIntervalMs,
			onUpdate: (newDate) => {
				const prevSelection = this.getSelectionSnapshot(id);
				const newSelection: unknown = select?.(newDate) ?? newDate;
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
	// gives us assurance about correctness). And for (1), useEffectEvent will
	// be initialized with whatever callback you give it on mount. So for the
	// mounting render alone, it's safe to call a useEffectEvent callback from
	// inside a render.
	const stableSelect = useEffectEvent((date: Date): T => {
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
