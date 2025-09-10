import {
	createContext,
	type FC,
	type PropsWithChildren,
	useCallback,
	useContext,
	useEffect,
	useId,
	useMemo,
	useState,
	useSyncExternalStore,
} from "react";
import { newReadonlyDate, noOp, TimeSync } from "utils/TimeSync";
import { useEffectEvent } from "./hookPolyfills";

export const REFRESH_IDLE = Number.POSITIVE_INFINITY;
export const REFRESH_ONE_SECOND: number = 1_000;
export const REFRESH_ONE_MINUTE = 60 * REFRESH_ONE_SECOND;
export const REFRESH_ONE_HOUR = 60 * REFRESH_ONE_MINUTE;

export type InitialDate = Date | (() => Date);

/**
 * @todo 2025-08-29 - This isn't 100% correct, but for the initial
 * implementation, we're going to assume that no one is going to be monkey-
 * patching custom symbol keys or non-enumerable keys onto built-in types (even
 * though this sort of already happens in the standard library)
 *
 * @todo 2025-09-02 - This function doesn't have any cycle detection. That
 * should be added at some point
 */
function structuralMerge<T = unknown>(oldValue: T, newValue: T): T {
	if (oldValue === newValue) {
		return oldValue;
	}

	// Making this the first major comparison, because realistically, a lot of
	// values are likely to be dates, since that's what you get when a custom
	// transformation isn't specified
	if (newValue instanceof Date) {
		if (!(oldValue instanceof Date)) {
			return newValue;
		}
		if (newValue.getMilliseconds() === oldValue.getMilliseconds()) {
			return oldValue;
		}
		return newValue;
	}

	switch (typeof newValue) {
		// If the new value is a primitive, we don't actually need to check the
		// old value at all. We can just return the new value directly, and have
		// JS language semantics take care of the rest
		case "boolean":
		case "number":
		case "bigint":
		case "string":
		case "undefined":
		case "symbol": {
			return newValue;
		}

		// If the new value is a function, we don't have a way of checking
		// whether the new function and old function are fully equivalent. While
		// we can stringify the function bodies and compare those, we have no
		// way of knowing if they're from the same execution context or have the
		// same closure values. Have to err on always returning the new value
		case "function": {
			return newValue;
		}

		case "object": {
			if (newValue === null || typeof oldValue !== "object") {
				return newValue;
			}
		}
	}

	if (Array.isArray(newValue)) {
		if (!Array.isArray(oldValue)) {
			return newValue;
		}

		const allMatch =
			oldValue.length === newValue.length &&
			oldValue.every((el, i) => el === newValue[i]);
		if (allMatch) {
			return oldValue;
		}
		const remapped = newValue.map((el, i) => structuralMerge(oldValue[i], el));
		return remapped as T;
	}

	const oldRecast = oldValue as Readonly<Record<string | symbol, unknown>>;
	const newRecast = newValue as Readonly<Record<string | symbol, unknown>>;

	const newStringKeys = Object.getOwnPropertyNames(newRecast);

	// If the new object has non-enumerable keys, there's not really much we can
	// do at a generic level to clone the object. So we have to return it out
	// unchanged
	const hasNonEnumerableKeys =
		newStringKeys.length !== Object.keys(newRecast).length;
	if (hasNonEnumerableKeys) {
		return newValue;
	}

	const newKeys = [
		...newStringKeys,
		...Object.getOwnPropertySymbols(newRecast),
	];

	const keyCountsMatch =
		newKeys.length ===
		Object.getOwnPropertyNames(oldRecast).length +
			Object.getOwnPropertySymbols(oldRecast).length;
	const allMatch =
		keyCountsMatch && newKeys.every((k) => oldRecast[k] === newRecast[k]);
	if (allMatch) {
		return oldValue;
	}

	const updated = { ...newRecast };
	for (const key of newKeys) {
		updated[key] = structuralMerge(oldRecast[key], newRecast[key]);
	}
	return updated as T;
}

type ReactTimeSyncInitOptions = Readonly<{
	initialDate: Date | (() => Date);
	isSnapshot: boolean;
}>;

type TransformCallback<T> = (
	state: Date,
) => T extends Promise<unknown> ? never : T extends void ? never : T;

type ReactSubscriptionHandshake = Readonly<{
	componentId: string;
	targetRefreshIntervalMs: number;
	transform: TransformCallback<unknown>;
	onReactStateSync: () => void;
}>;

// All properties in this type are mutable on purpose
type TransformationEntry = {
	unsubscribe: () => void;
	cachedTransformation: unknown;
};

/**
 * The main conceit for this file is that all of the core state is stored in a
 * global-ish instance of ReactTimeSync, and then useTimeSync and
 * useTimeSyncState control the class via React hooks and lifecycle behavior.
 *
 * Because you can't share generics at a module level, the class uses a bunch
 * of `unknown` types to handle storing arbitrary data.
 */
class ReactTimeSync {
	static readonly #stalenessThresholdMs = 250;

	// Each string key is a globally-unique ID that identifies a specific React
	// component instance (i.e., two React Fiber entries made from the same
	// function component should have different IDs)
	readonly #entries: Map<string, TransformationEntry>;
	readonly #timeSync: TimeSync;
	#isProviderMounted: boolean;
	#invalidationIntervalId: NodeJS.Timeout | number | undefined;

	constructor(options?: Partial<ReactTimeSyncInitOptions>) {
		const { initialDate: init, isSnapshot } = options ?? {};

		this.#isProviderMounted = true;
		this.#invalidationIntervalId = undefined;
		this.#entries = new Map();

		const initialDate = typeof init === "function" ? init() : init;
		this.#timeSync = new TimeSync({ initialDate, isSnapshot });
	}

	// Only safe to call inside a render with useSyncExternalStore
	getDateSnapshot(): Date {
		return this.#timeSync.getStateSnapshot();
	}

	// Always safe to call inside a render
	getTimeSync(): TimeSync {
		return this.#timeSync;
	}

	subscribe(rsh: ReactSubscriptionHandshake): () => void {
		if (!this.#isProviderMounted) {
			return noOp;
		}

		const {
			componentId,
			targetRefreshIntervalMs,
			onReactStateSync,
			transform,
		} = rsh;

		/**
		 * This if statement is handling two situations:
		 * 1. The activeEntry already exists because it was pre-seeded with data
		 *    (in which case, the existing transformation is safe to reuse)
		 * 2. An unsubscribe didn't trigger before setting up a new subscription
		 *    for the same component instance. This should be impossible, but
		 *    better to be defensive
		 */
		let activeEntry = this.#entries.get(componentId);
		if (activeEntry !== undefined) {
			activeEntry.unsubscribe();
			activeEntry.unsubscribe = noOp;
		} else {
			activeEntry = {
				unsubscribe: noOp,
				cachedTransformation: transform(this.getDateSnapshot()),
			};
			this.#entries.set(componentId, activeEntry);
		}

		const unsubscribeFromRootSync = this.#timeSync.subscribe({
			targetRefreshIntervalMs,
			onUpdate: (newDate) => {
				const entry = this.#entries.get(componentId);
				if (entry === undefined) {
					return;
				}

				const oldState = entry.cachedTransformation;
				const newState = transform(newDate);
				const merged = structuralMerge(oldState, newState);

				if (oldState !== merged) {
					entry.cachedTransformation = merged;
					onReactStateSync();
				}
			},
		});

		const unsubscribe = (): void => {
			unsubscribeFromRootSync();
			this.#entries.delete(componentId);
		};
		activeEntry.unsubscribe = unsubscribe;

		// Regardless of how the subscription happened, update all other
		// subscribers to get them in sync with the newest state
		const shouldInvalidateDate =
			newReadonlyDate().getTime() -
				this.#timeSync.getStateSnapshot().getTime() >
			ReactTimeSync.#stalenessThresholdMs;
		if (shouldInvalidateDate) {
			void this.#timeSync.invalidateStateSnapshot({
				// This is normally a little risky, but because of how the
				// onUpdate callback above is defined, dispatching a
				// subscription update doesn't always trigger a re-render
				notificationBehavior: "always",
			});
		}

		return unsubscribe;
	}

	updateComponentState(componentId: string, newValue: unknown): void {
		if (!this.#isProviderMounted) {
			return;
		}

		// If we're invalidating the transformation before a subscription has
		// been set up, then we almost definitely need to pre-seed the class
		// with data. We want to avoid callingredundant transformations since we
		// don't know in advance how expensive transformations can get
		const entry = this.#entries.get(componentId);
		if (entry === undefined) {
			this.#entries.set(componentId, {
				unsubscribe: noOp,
				cachedTransformation: newValue,
			});
			return;
		}

		// It's expected that whichever hook is calling this method will have
		// already created the new value via structural sharing. Calling this
		// again should just return out the old state. But if something goes
		// wrong, having an extra merge step removes some potential risks
		const merged = structuralMerge(entry.cachedTransformation, newValue);
		entry.cachedTransformation = merged;
	}

	// Always safe to call inside a render
	getComponentSnapshot<T>(componentId: string): T {
		// It's super important that we have this function be set up to always
		// return a value, because on mount, useSyncExternalStore will call the
		// state getter before the subscription has been set up
		const prev = this.#entries.get(componentId);
		if (prev !== undefined) {
			return prev.cachedTransformation as T;
		}

		const latestDate = this.#timeSync.getStateSnapshot();
		return latestDate as T;
	}

	onComponentMount(): void {
		if (!this.#isProviderMounted) {
			return;
		}

		void this.#timeSync.invalidateStateSnapshot({
			notificationBehavior: "onChange",
		});
	}

	onProviderMount(): () => void {
		if (!this.#isProviderMounted) {
			return noOp;
		}

		// Periodially invalidate the state, so that even if all subscribers
		// have really slow refresh intervals, when a new component gets
		// mounted, it will be guaranteed to have "fresh-ish" data.
		this.#invalidationIntervalId = setTimeout(() => {
			this.#timeSync.invalidateStateSnapshot({
				stalenessThresholdMs: ReactTimeSync.#stalenessThresholdMs,
				notificationBehavior: "never",
			});
		}, ReactTimeSync.#stalenessThresholdMs);

		const cleanup = () => {
			this.#isProviderMounted = false;
			clearTimeout(this.#invalidationIntervalId);
			this.#invalidationIntervalId = undefined;
			this.#timeSync.dispose();
			this.#entries.clear();
		};
		return cleanup;
	}
}

const reactTimeSyncContext = createContext<ReactTimeSync | null>(null);

function useReactTimeSync(): ReactTimeSync {
	const reactTs = useContext(reactTimeSyncContext);
	if (reactTs === null) {
		throw new Error(
			`Must call TimeSync hook from inside ${TimeSyncProvider.name}`,
		);
	}
	return reactTs;
}

type TimeSyncProviderProps = PropsWithChildren<{
	initialDate?: InitialDate;
	isSnapshot?: boolean;
}>;

export const TimeSyncProvider: FC<TimeSyncProviderProps> = ({
	children,
	initialDate,
	isSnapshot = false,
}) => {
	const [readonlyReactTs] = useState(() => {
		return new ReactTimeSync({ initialDate, isSnapshot });
	});

	useEffect(() => {
		return readonlyReactTs.onProviderMount();
	}, [readonlyReactTs]);

	return (
		<reactTimeSyncContext.Provider value={readonlyReactTs}>
			{children}
		</reactTimeSyncContext.Provider>
	);
};

/**
 * Provides direct access to the TimeSync instance being dependency-injected
 * throughout the React application.
 *
 * This lets you a component receive the active TimeSync without binding it to
 * React lifecycles (essentially making it ref state).
 *
 * The instance methods generally shouldn't be called during a render, unless
 * you take the pains to make all the logic render-safe. If you need to
 * bind state updates to TimeSync, consider using `useTimeSyncState` instead,
 * which handles all those safety concerns for you automatically.
 */
function useTimeSync(): TimeSync {
	const reactTs = useReactTimeSync();
	return reactTs.getTimeSync();
}

/**
 * useTimeSync is a core part of the design for TimeSync, but because no
 * components need it yet, we're doing this to suppress Knip warnings.
 */
const _unused = useTimeSync;

// Even though this is a really simple function, keeping it defined outside the
// hook helps a lot with making sure useSyncExternalStore doesn't re-sync too
// often
function identity<T>(value: T): T {
	return value;
}

type ReactSubscriptionCallback = (notifyReact: () => void) => () => void;

type UseTimeSyncOptions<T> = Readonly<{
	/**
	 * The ideal interval of time, in milliseconds, that defines how often the
	 * hook should refresh with the newest state value from TimeSync.
	 *
	 * Note that a refresh is not the same as a re-render. If the hook is
	 * refreshed with a new datetime, but its transform callback produces the
	 * same value as before, the hook will skip re-rendering.
	 *
	 * The hook reserves the right to refresh MORE frequently than the
	 * specified interval if it would guarantee that the hook does not get out
	 * of sync with other useTimeSync users. This removes the risk of screen
	 * tearing.
	 */
	targetIntervalMs: number;

	/**
	 * Allows you to transform any Date values received from the TimeSync
	 * class. If provided, the hook will return the result of calling the
	 * `transform` callback instead of the main Date state.
	 *
	 * `transform` works almost exactly like the `select` callback in React
	 * Query's `useQuery` hook. That is:
	 * 1. Inline functions are always re-run during re-renders to avoid stale
	 *    data issues.
	 * 2. `transform` does not use dependency arrays directly, but if it is
	 *    memoized via `useCallback`, it will only re-run during a re-render if
	 *    `useCallback` got invalidated or the date state changed.
	 * 3. When TimeSync dispatches a new date update, it will run the latest
	 *    `transform` callback. If the result has not changed (comparing by
	 *    value), the component will try to bail out of re-rendering. At that
	 *    stage, the component will only re-render if a parent component
	 *    re-renders
	 *
	 * `transform` callbacks must not be async. The hook will error out at the
	 * type level if you provide one by mistake.
	 */
	transform?: TransformCallback<T>;
}>;

/**
 * Lets you bind your React component's state to a TimeSync's time management
 * logic.
 *
 * When the hook is called for the first time, the date state is guaranteed to
 * be updated, no matter how many subscribers were set up before, or what
 * intervals they subscribed with.
 *
 * Note that any component mounted with this hook will re-render under two
 * situations:
 * 1. A state update was dispatched via the TimeSync's normal time update logic.
 * 2. If a component was mounted for the first time with a fresh date, all other
 *    components will be "refreshed" to use the same date as well. This is to
 *    avoid stale date issues, and it will happen even if all other subscribers
 *    were subscribed with an interval of positive infinity.
 */
export function useTimeSyncState<T = Date>(options: UseTimeSyncOptions<T>): T {
	const { targetIntervalMs, transform } = options;
	const activeTransform = (transform ?? identity) as TransformCallback<T>;

	// This is an abuse of the useId API, but because it gives us an ID that is
	// uniquely associated with the current component instance, we can use it to
	// differentiate between multiple instances of the same function component
	// subscribing to useTimeSyncState
	const hookId = useId();
	const reactTs = useReactTimeSync();

	// getSnap should be 100% stable for the entire component lifetime. Note
	// that because of how React lifecycles work, getSnap will always return
	// the TimeSync's current Date object on the mounting render (without ever
	// applying any transformations). This is expected, and the rest of the hook
	// logic ensures that it will be intercepted before being returned to
	// consumers
	const getSnap = useCallback(
		() => reactTs.getComponentSnapshot<T>(hookId),
		[reactTs, hookId],
	);

	// This is a little cursed, but to avoid chicken-and-the-egg problems when
	// dealing with React lifecycles, we need to manually split
	// useSyncExternalStore's logic in half. We need to get the state
	// immediately, and then delay calling the actual hook (and the timing for
	// when the subscription actually gets set up) to be after all the other
	// effects have been mounted. This is technically breaking the rules, but
	// as long as we call useSyncExternalStore at some point with this exact
	// same callback, there's no risk of data getting out of sync
	const cachedTransformation = getSnap();

	// There's some trade-offs with this memo (notably, if the consumer passes
	// in an inline transform callback, the memo result will be invalidated on
	// every single render). But it's the *only* way to give the consumer the
	// option of memoizing expensive transformations at the render level without
	// polluting the hook's API with super-fragile dependency array logic
	const newTransformation = useMemo(() => {
		// Since we're breaking the rules slightly below, we need to opt this
		// function out of being compiled by the React Compiler to make sure it
		// doesn't compile the function the wrong way
		"use no memo";

		// This state getter is technically breaking the React rules, because
		// we're getting a mutable value while in a render without binding it to
		// state. But it's "pure enough", and the useSyncExternalStore logic for
		// the transformation snapshots ensures that things won't actually get
		// out of sync. We can't subscribe to the date itself, because then we
		// lose the ability to re-render only on changed transformations
		const latestDate = reactTs.getDateSnapshot();
		return activeTransform(latestDate);
	}, [reactTs, activeTransform]);

	// Making sure to merge the results so that the hook interfaces well with
	// memoization and effects outside of this hook
	const merged = useMemo(
		() => structuralMerge(cachedTransformation, newTransformation),
		[cachedTransformation, newTransformation],
	);

	useEffect(() => {
		reactTs.updateComponentState(hookId, merged);
	}, [reactTs, hookId, merged]);

	// We want to make sure that the mounting effect fires after the initial
	// transform invalidation to minimize the risks of React being over-notified
	// of state updates
	useEffect(() => {
		reactTs.onComponentMount();
	}, [reactTs]);

	// Because of how React lifecycles work, this effect event callback should
	// never be called from inside render logic. It will *always* give you
	// stale data after the very first render.
	const externalTransform = useEffectEvent(activeTransform);

	// Dependency array elements listed for correctness, but the only value that
	// can change on re-renders (which React guarantees) is the target interval
	const subscribe = useCallback<ReactSubscriptionCallback>(
		(notifyReact) => {
			return reactTs.subscribe({
				componentId: hookId,
				targetRefreshIntervalMs: targetIntervalMs,
				transform: externalTransform,
				onReactStateSync: notifyReact,
			});
		},
		[reactTs, hookId, externalTransform, targetIntervalMs],
	);

	// We already have the actual state value at this point, so we just need to
	// wire up useSyncExternalStore to satisfy the hook API and give ourselves
	// state-binding guarantees
	void useSyncExternalStore(subscribe, getSnap);

	return merged;
}
