import {
	createContext,
	type FC,
	type ReactNode,
	useCallback,
	useContext,
	useId,
	useInsertionEffect,
	useLayoutEffect,
	useMemo,
	useReducer,
	useRef,
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

	/**
	 * Used to "batch" up multiple calls to this.syncAllSubscribersOnMount after
	 * a given render phase, and make sure that no matter how many component
	 * instances are newly mounted, the logic only fires once. This logic is
	 * deeply dependent on useLayoutEffect's API, which blocks DOM painting
	 * until all the queued layout effects fire
	 *
	 * useAnimationFrame gives us a way to detect when all our layout effects
	 * have finished processing and have produced new UI on screen.
	 *
	 * This pattern for detecting when layout effects fire is normally NOT safe,
	 * but because all the mounting logic is synchronous, that gives us
	 * guarantees that when a new animation frame is available, there will be
	 * no incomplete/in-flight effects from the hook. We don't want to throttle
	 * calls, because rapid enough updates could outpace the throttle threshold,
	 * which could cause some updates to get dropped
	 *
	 * @todo Double-check that cascading layout effects does cause the painting
	 * to be fully blocked until everything settles down.
	 */
	#batchMountUpdateId: number | undefined;

	constructor(options?: Partial<ReactTimeSyncInitOptions>) {
		const { initialDate: init, isSnapshot } = options ?? {};

		this.#isProviderMounted = true;
		this.#invalidationIntervalId = undefined;
		this.#entries = new Map();

		const initialDate = typeof init === "function" ? init() : init;
		this.#timeSync = new TimeSync({ initialDate, isSnapshot });
	}

	// Only safe to call inside a render that is bound to useSyncExternalStore
	// in some way
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

	syncAllSubscribersOnMount(): void {
		/**
		 * It's hokey to think about, but this logic *should* still work in the
		 * event that layout effects cause other useTimeSync consumers to mount.
		 *
		 * Even though a layout effect might produce 2+ new render passes
		 * before paint (each with their own layout effects), React will still
		 * be in control of the event loop the entire time. There's no way for
		 * any other TimeSync logic to fire or update state. So while the extra
		 * mounting components will technically never be able to dispatch their
		 * own syncs, we can reuse the state produced from the original sync.
		 *
		 * @todo There is a chance that if any given render pass takes an
		 * especially long time, and we have subscribers that need updates
		 * faster than every second, reusing an old snapshot across multiple
		 * cascading layout renders might not be safe. But better to hold off on
		 * handling that edge case for now
		 */
		if (!this.#isProviderMounted || this.#batchMountUpdateId !== undefined) {
			return;
		}

		this.#batchMountUpdateId = requestAnimationFrame(() => {
			this.#batchMountUpdateId = undefined;
		});

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

			if (this.#batchMountUpdateId !== undefined) {
				cancelAnimationFrame(this.#batchMountUpdateId);
				this.#batchMountUpdateId = undefined;
			}
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

type TimeSyncProviderProps = Readonly<{
	initialDate?: InitialDate;
	isSnapshot?: boolean;
	children?: ReactNode;
}>;

export const TimeSyncProvider: FC<TimeSyncProviderProps> = ({
	children,
	initialDate,
	isSnapshot = false,
}) => {
	const [readonlyReactTs] = useState(() => {
		return new ReactTimeSync({ initialDate, isSnapshot });
	});

	// This is a super, super niche use case, but we need to make ensure the
	// effect for setting up the provider mounts before the effects in the
	// individual hook consumers. Because the hooks use useLayoutEffect, which
	// already has higher priority than useEffect, and because effects always
	// fire from the bottom up in the UI tree, the only option is to use the one
	// effect type that has faster firing priority than useLayoutEffect
	useInsertionEffect(() => {
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

// Even though this is a really simple function, keeping it defined outside
// useTimeSyncState helps with render performance, and helps stabilize a bunch
// of values in the hook when you're not doing transformations
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
	 * refreshed with a new datetime, but the state for the component itself has
	 * not changed, the hook will bail out of re-rendering.
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
 * Lets you bind your React component's state to a TimeSync's time subscription
 * logic.
 *
 * When the hook is called for the first time, the date state it works with is
 * guaranteed to be within one second of the current time, not the date state
 * that was used for the last notification (if any happened at all).
 *
 * Note that any component mounted with this hook will re-render under two
 * situations:
 * 1. A state update was dispatched via the TimeSync's normal time update logic.
 * 2. If a component was mounted for the first time with a fresh date, all other
 *    components will be "refreshed" to use the same date as well. This is to
 *    avoid stale date issues, and will happen even if all other subscribers
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

	// getSnap should be 100% stable for the entire component lifetime to
	// minimize unnecessary function calls for useSyncExternalStore. Note that
	// because of how React lifecycles work, getSnap will always return the
	// TimeSync's current Date object on the mounting render (without ever
	// applying any transformations). This is expected, and the rest of the hook
	// logic ensures that it will be intercepted before being returned to
	// consumers
	const getSnap = useCallback(
		() => reactTs.getComponentSnapshot<T>(hookId),
		[reactTs, hookId],
	);

	// Because of how React lifecycles work, this effect event callback should
	// never be called from inside render logic. While called in a re-render, it
	// will *always* give you stale date, but it will be correct by the time
	// the external system needs to use the function
	const externalTransform = useEffectEvent(activeTransform);

	// This is a hack to deal with some of the timing issues when dealing with
	// useSyncExternalStore. The subscription logic fires at useEffect priority,
	// which (1) is too slow since we have layout effects, and (2) even if the
	// subscription fired at layout effect speed, we actually need to delay when
	// it gets set up so that other layout effects can fire first. This is
	// *very* wacky, but satisfies all the React rules, and avoids a bunch of
	// chicken-and-the-egg problems when dealing with React lifecycles and state
	// sometimes not being defined
	const ejectedNotifyRef = useRef<() => void>(noOp);
	const subscribeWithEjection = useCallback<ReactSubscriptionCallback>(
		(notifyReact) => {
			ejectedNotifyRef.current = notifyReact;
			return noOp;
		},
		[],
	);

	/**
	 * Important bits of context that the React docs don't cover:
	 *
	 * useSyncExternalStore has two distinct phases when it mounts:
	 * 1. The state getter runs first in the render itself, and is called twice
	 *    to guarantee that the snapshot is stable.
	 * 2. Once the render completes, the subscription fires at useEffect
	 *    priority (meaning that layout effects can out-race it)
	 *
	 * Also, both functions will re-run every time their function references
	 * change, which is why both callbacks are memoized. We don't want
	 * subscriptions torn down and rebuilt each render.
	 */
	const cachedTransformation = useSyncExternalStore(
		subscribeWithEjection,
		getSnap,
	);

	/**
	 * @todo Figure out if I could actually update the definition for
	 * getComponentSnapshot to put the date value on the snapshot itself, but
	 * just don't notify React when the state changes
	 */
	const todo = void "I dunno, try it";

	// There's some trade-offs with this memo (notably, if the consumer passes
	// in an inline transform callback, the memo result will be invalidated on
	// every single render). But it's the *only* way to give the consumer the
	// option of memoizing expensive transformations at the render level without
	// polluting the hook's API with super-fragile dependency array logic
	const newTransformation = useMemo(() => {
		// Since this function is used to break the React rules slightly, we
		// need to opt this function out of being compiled by the React Compiler
		// to make sure it doesn't compile the function the wrong way
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

	// Because this is a layout effect, it's guaranteed to fire before the
	// subscription logic, even though the subscription was registered first.
	// This lets us cut back on redoing computations that were already handled
	// in the render
	useLayoutEffect(() => {
		reactTs.updateComponentState(hookId, merged);
	}, [reactTs, hookId, merged]);

	// For correctness, because the hook notifies all subscribers of a potential
	// state change on mount, we need to make sure that the subscription gets
	// set up with a working state setter callback. This can be used until the
	// low-priority useSyncExternalStore subscription fires. If all goes well,
	// it shouldn't ever be needed, but this truly ensures that the various
	// systems can't get out of sync with each other. We only use this on the
	// mounting render because the notifyReact callback is better in all ways.
	// It's much more fine-grained and is actively associated with the state
	// lifecycles for the useSyncExternalStore hook
	const [, fallbackStateSync] = useReducer(
		(dummyForceRerenderState) => !dummyForceRerenderState,
		false,
	);

	// There's a lot of dependencies here, but the only cue for invalidating the
	// subscription should be the target interval changing
	useLayoutEffect(() => {
		return reactTs.subscribe({
			componentId: hookId,
			targetRefreshIntervalMs: targetIntervalMs,
			transform: externalTransform,
			onReactStateSync: () => {
				if (ejectedNotifyRef.current === noOp) {
					fallbackStateSync();
				} else {
					ejectedNotifyRef.current();
				}
			},
		});
	}, [reactTs, hookId, externalTransform, targetIntervalMs]);

	// This is the one case where we're using useLayoutEffect for its intended
	// purpose, but it's also the reason why we have to worry about effect
	// firing speed. Because the mounting logic is able to trigger state
	// updates, we need to fire them before paint to make sure that we don't get
	// screen flickering
	useLayoutEffect(() => {
		reactTs.syncAllSubscribersOnMount();
	}, [reactTs]);

	return merged;
}
