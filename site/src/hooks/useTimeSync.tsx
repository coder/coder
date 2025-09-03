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
import {
	defaultOptions,
	newReadonlyDate,
	noOp,
	TimeSync,
} from "utils/TimeSync";
import { useEffectEvent } from "./hookPolyfills";

export const REFRESH_IDLE = Number.POSITIVE_INFINITY;
export const REFRESH_ONE_SECOND: number = 1_000;
export const REFRESH_ONE_MINUTE = 60 * 1_000;
export const REFRESH_ONE_HOUR = 60 * 60 * 1_000;

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
	initialDatetime: Date;
	minimumRefreshIntervalMs: number;
	disableUpdates: boolean;
}>;

type TransformCallback<T> = (
	state: Date,
) => T extends Promise<unknown> ? never : T extends void ? never : T;

type TransformationHandshake = Readonly<{
	componentId: string;
	targetRefreshIntervalMs: number;
	onStateUpdate: () => void;
	transform: TransformCallback<unknown>;
}>;

type TransformationEntry = {
	readonly unsubscribe: () => void;
	cachedTransformation: unknown;
};

const staleStateThresholdMs = 100;

class ReactTimeSync {
	// Each string is a globally-unique ID that identifies a specific React
	// component instance (i.e., two React Fiber entries made from the same
	// function component should have different IDs)
	readonly #entries: Map<string, TransformationEntry>;
	readonly #timeSync: TimeSync;
	#isProviderMounted: boolean;
	#hasPendingUpdates: boolean;

	constructor(options?: Partial<ReactTimeSyncInitOptions>) {
		const {
			initialDatetime = defaultOptions.initialDatetime,
			minimumRefreshIntervalMs = defaultOptions.minimumRefreshIntervalMs,
		} = options ?? {};

		this.#isProviderMounted = true;
		this.#hasPendingUpdates = false;
		this.#entries = new Map();
		this.#timeSync = new TimeSync({
			initialDatetime,
			minimumRefreshIntervalMs,
		});
	}

	subscribeToTransformations(th: TransformationHandshake): () => void {
		if (!this.#isProviderMounted) {
			return noOp;
		}

		const { componentId, targetRefreshIntervalMs, onStateUpdate, transform } =
			th;

		const prevEntry = this.#entries.get(componentId);
		if (prevEntry !== undefined) {
			prevEntry.unsubscribe();
			this.#entries.delete(componentId);
		}

		const unsubscribe = this.#timeSync.subscribe({
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
					onStateUpdate();
				}
			},
		});

		const latestSyncState = this.#timeSync.getStateSnapshot();
		const newEntry: TransformationEntry = {
			unsubscribe,
			/**
			 * @todo 2025-08-29 - There is one unfortunate behavior with the
			 * current subscription logic. Because of how React lifecycles work,
			 * each new component instance needs to call the transform callback
			 * twice on setup. You need to call it once from the render, and
			 * again from the subscribe method.
			 *
			 * Trying to fix this got really nasty, and caused a bunch of
			 * chicken-and-the-egg problems. Luckily, most transformations
			 * should be super cheap, which should buy us some time to get this
			 * fixed.
			 */
			cachedTransformation: transform(latestSyncState),
		};
		this.#entries.set(componentId, newEntry);

		return () => {
			newEntry.unsubscribe();
			this.#entries.delete(componentId);
		};
	}

	invalidateTransformation(componentId: string, newValue: unknown): void {
		if (!this.#isProviderMounted) {
			return;
		}

		const entry = this.#entries.get(componentId);
		if (entry === undefined) {
			return;
		}

		// It's expected that whichever hook is calling this method will have
		// already created the new value via structural sharing. Calling this
		// again should just return out the old state. But if something goes
		// wrong, having an extra merge step removes some potential risks
		const merged = structuralMerge(entry.cachedTransformation, newValue);
		entry.cachedTransformation = merged;
	}

	// It's super important that we have this function be set up to always
	// return a value, because on mount, useSyncExternalStore will call the
	// state getter before the subscription has been set up
	getTransformationSnapshot<T>(componentId: string): T {
		const prev = this.#entries.get(componentId);
		if (prev !== undefined) {
			return prev.cachedTransformation as T;
		}

		const latestDate = this.#timeSync.getStateSnapshot();
		return latestDate as T;
	}

	getLatestDate(): Date {
		let snap = this.#timeSync.getStateSnapshot();

		const shouldInvalidate =
			this.#isProviderMounted &&
			newReadonlyDate().getTime() - snap.getTime() > staleStateThresholdMs;
		if (shouldInvalidate) {
			snap = this.#timeSync.invalidateStateSnapshot({
				notifyAfterUpdate: false,
			});
			this.#hasPendingUpdates = true;
		}

		return snap;
	}

	getTimeSync(): TimeSync {
		return this.#timeSync;
	}

	onComponentMount(): void {
		if (!this.#isProviderMounted || !this.#hasPendingUpdates) {
			return;
		}
		void this.#timeSync.invalidateStateSnapshot({ notifyAfterUpdate: true });
	}

	onProviderUnmount(): void {
		if (!this.#isProviderMounted) {
			return;
		}
		this.#timeSync.dispose();
		this.#entries.clear();
		this.#isProviderMounted = false;
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
	initialDatetime?: Date;
	minimumRefreshIntervalMs?: number;
}>;

export const TimeSyncProvider: FC<TimeSyncProviderProps> = ({
	children,
	initialDatetime,
	minimumRefreshIntervalMs,
}) => {
	const [readonlyReactTs] = useState(() => {
		return new ReactTimeSync({ initialDatetime, minimumRefreshIntervalMs });
	});

	useEffect(() => {
		return () => readonlyReactTs.onProviderUnmount();
	}, [readonlyReactTs]);

	return (
		<reactTimeSyncContext.Provider value={readonlyReactTs}>
			{children}
		</reactTimeSyncContext.Provider>
	);
};

/**
 * Provides access to the TimeSync instance currently being dependency-injected
 * throughout the application. This lets you set up manual subscriptions that
 * don't need to be directly tied to React's lifecycles.
 *
 * This hook shouldn't be necessary the majority of the time. If you need to
 * bind state updates to TimeSync, consider using `useTimeSyncState` instead.
 *
 * This hook is a core part of the design for TimeSync, but because no
 * components need it yet, it's defined with an _ to make Knip happy.
 */
function _useTimeSync(): TimeSync {
	const reactTs = useReactTimeSync();
	return reactTs.getTimeSync();
}

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

export function useTimeSyncState<T = Date>(options: UseTimeSyncOptions<T>): T {
	const { targetIntervalMs, transform } = options;
	const activeTransform = (transform ?? identity) as TransformCallback<T>;

	const hookId = useId();
	const reactTs = useReactTimeSync();

	// Because of how React lifecycles work, the effect event callback should
	// never be called from inside render logic. It will *always* give you
	// stale data after the very first render.
	const externalTransform = useEffectEvent(activeTransform);
	const subscribe = useCallback<ReactSubscriptionCallback>(
		(notifyReact) => {
			return reactTs.subscribeToTransformations({
				componentId: hookId,
				targetRefreshIntervalMs: targetIntervalMs,
				transform: externalTransform,
				onStateUpdate: notifyReact,
			});
		},
		[reactTs, hookId, externalTransform, targetIntervalMs],
	);
	const getSnap = useCallback(
		() => reactTs.getTransformationSnapshot<T>(hookId),
		[reactTs, hookId],
	);

	/**
	 * This is how useSyncExternalStore's on-mount logic works (which the React
	 * Docs doesn't cover at all):
	 * 1. It calls the snapshot getter function twice to validate that the
	 *    state has a stable reference.
	 * 2. The hook stores the second return value as state and returns it out
	 *    immediately
	 * 3. Once the render has finished, React will set up the subscription. This
	 *    happens at useEffect speed (so slowest priority).
	 *
	 * Because of this, cachedTransformation will always be a Date object on
	 * mount (no matter what transformation was provided), because nothing will
	 * have been set up to uniquely identify the component instance for the
	 * ReactTimeSync class. This is expected, and the useMemo calls later in the
	 * hook make sure that it doesn't get returned out if a transformation is
	 * specified.
	 */
	const cachedTransformation = useSyncExternalStore(subscribe, getSnap);

	// There's some trade-offs with this memo (notably, if the consumer passes
	// in an inline transform callback, the memo result will be invalidated on
	// every single render). But it's the *only* way to give the consumer the
	// option of memoizing expensive transformations at the render level without
	// polluting the hook's API with super-fragile dependency array logic
	const newTransformation = useMemo(() => {
		// Calling reactTs.getDateSnapshot like this is technically breaking the
		// React rules, but we need to make sure that if activeTransform changes
		// on re-renders, and it's been a while since the cached transformation
		// changed, we don't have drastically outdated date state. We could also
		// subscribe to the date itself, but that makes it impossible to prevent
		// unnecessary re-renders on subscription updates
		const latestDate = reactTs.getLatestDate();
		return activeTransform(latestDate);
	}, [reactTs, activeTransform]);

	// Making sure to merge the results so that the hook interfaces well with
	// memoization and effects outside of this hook
	const merged = useMemo(
		() => structuralMerge(cachedTransformation, newTransformation),
		[cachedTransformation, newTransformation],
	);

	useEffect(() => {
		reactTs.invalidateTransformation(hookId, merged);
	}, [reactTs, hookId, merged]);

	// We want to make sure that the mounting effect fires after the initial
	// transform invalidation to minimize the risks of React being over-notified
	// of state updates
	useEffect(() => {
		reactTs.onComponentMount();
	}, [reactTs]);

	return merged;
}
