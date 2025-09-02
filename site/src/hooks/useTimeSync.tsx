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
	noOp,
	type OnUpdate,
	TimeSync,
	defaultOptions as timeSyncDefaultOptions,
} from "utils/TimeSync";
import { useEffectEvent } from "./hookPolyfills";

export const REFRESH_IDLE = Number.POSITIVE_INFINITY;
export const REFRESH_ONE_SECOND: number = 1_000;
export const REFRESH_ONE_MINUTE = 60 * 1_000;
export const REFRESH_ONE_HOUR = 60 * 60 * 1_000;

function structuralMerge(oldValue: unknown, newValue: unknown): unknown {
	if (oldValue === newValue) {
		return oldValue;
	}

	const newType = typeof newValue;
	switch (newType) {
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
	}
	if (newType === null || typeof oldValue !== "object") {
		return newValue;
	}

	/**
	 * @todo 2025-08-29 - This isn't 100% correct, but for the initial
	 * implementation, we're going to assume that no one is going to be monkey-
	 * patching custom symbol keys or non-enumerable keys onto built-in types
	 * (even though this sort of already happens in the standard library)
	 */
	if (newValue instanceof Date) {
		if (!(oldValue instanceof Date)) {
			return newValue;
		}
		if (newValue.getMilliseconds() === oldValue.getMilliseconds()) {
			return oldValue;
		}
		return newValue;
	}

	if (Array.isArray(newValue)) {
		if (!Array.isArray(oldValue) || oldValue.length !== newValue.length) {
			return newValue;
		}
		const allMatch = oldValue.every((el, i) => el === newValue[i]);
		if (allMatch) {
			return oldValue;
		}
		return oldValue.map((el, i) => structuralMerge(el, newValue[i]));
	}

	const oldRecast = oldValue as Record<string | symbol, unknown>;
	const newRecast = newValue as Record<string | symbol, unknown>;
	const allKeys = [
		...Object.getOwnPropertyNames(newRecast),
		...Object.getOwnPropertySymbols(newRecast),
	];

	const newPropKeys = Object.getOwnPropertyNames(newValue);
	if (newPropKeys.length !== Object.getOwnPropertyNames(oldValue).length) {
		return newValue;
	}

	return newValue;
}

type ReactTimeSyncInitOptions = Readonly<{
	initialDatetime: Date;
	minimumRefreshIntervalMs: number;
	disableUpdates: boolean;
}>;

type TransformCallback<T> = (
	state: Date,
) => T extends Promise<unknown> ? never : T extends void ? never : T;

type SubscriptionHandshake = Readonly<{
	componentId: string;
	targetRefreshIntervalMs: number;
	onStateUpdate: () => void;
	transform: TransformCallback<unknown>;
}>;

type SubscriptionEntry = {
	readonly unsubscribe: () => void;
	lastTransformedValue: unknown;
};

const defaultReactOptions: ReactTimeSyncInitOptions = {
	disableUpdates: false,
	initialDatetime: timeSyncDefaultOptions.initialDatetime,
	minimumRefreshIntervalMs: timeSyncDefaultOptions.minimumRefreshIntervalMs,
};

class ReactTimeSync {
	readonly #entries = new Map<string, SubscriptionEntry>();
	#disposed = false;

	readonly #disableUpdates: boolean;
	readonly #timeSync: TimeSync;

	constructor(options?: Partial<ReactTimeSyncInitOptions>) {
		const {
			initialDatetime = defaultReactOptions.initialDatetime,
			minimumRefreshIntervalMs = defaultReactOptions.minimumRefreshIntervalMs,
			disableUpdates = defaultReactOptions.disableUpdates,
		} = options ?? {};

		this.#disableUpdates = disableUpdates;
		this.#timeSync = new TimeSync({
			initialDatetime,
			minimumRefreshIntervalMs,
		});
	}

	subscribe(sh: SubscriptionHandshake): () => void {
		if (this.#disposed) {
			return noOp;
		}

		const { componentId, targetRefreshIntervalMs, onStateUpdate, transform } =
			sh;

		const prevEntry = this.#entries.get(componentId);
		if (prevEntry !== undefined) {
			prevEntry.unsubscribe();
			this.#entries.delete(componentId);
		}

		// Even if updates are disabled, we still need to set up a subscription,
		// so that we satisfy the API for useSyncExternalStore. So if TimeSync
		// is disabled, we'll just pass in a no-op function instead
		let onUpdate: OnUpdate = noOp;
		if (!this.#disableUpdates) {
			onUpdate = (newDate) => {
				const entry = this.#entries.get(componentId);
				if (entry === undefined) {
					return;
				}

				const newState = transform(newDate);
				const merged = structuralMerge(entry.lastTransformedValue, newState);

				if (entry.lastTransformedValue !== merged) {
					entry.lastTransformedValue = merged;
					onStateUpdate();
				}
			};
		}

		const unsubscribe = this.#timeSync.subscribe({
			targetRefreshIntervalMs,
			onUpdate,
		});

		const latestSyncState = this.#timeSync.getStateSnapshot();
		const newEntry: SubscriptionEntry = {
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
			lastTransformedValue: transform(latestSyncState),
		};
		this.#entries.set(componentId, newEntry);

		return () => {
			newEntry.unsubscribe();
			this.#entries.delete(componentId);
		};
	}

	updateCachedTransformation(componentId: string, newValue: unknown): void {
		if (this.#disposed) {
			return;
		}

		const entry = this.#entries.get(componentId);
		if (entry === undefined) {
			return;
		}
		const updated = structuralMerge(entry.lastTransformedValue, newValue);
		entry.lastTransformedValue = updated;
	}

	onUnmount(): void {
		if (this.#disposed) {
			return;
		}
		this.#timeSync.dispose();
		this.#entries.clear();
		this.#disposed = true;
	}

	// This function *must* be defined as an arrow function so that we can pass
	// it directly into useSyncExternalStore. This not only removes the need to
	// make a bunch of arrow functions in the render, but keeping the memory
	// reference for the state getter 100% stable means that React can apply
	// more render optimizations
	getDateSnapshot = (): Date => {
		return this.#timeSync.getStateSnapshot();
	};
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
		return () => readonlyReactTs.onUnmount();
	}, [readonlyReactTs]);

	return (
		<reactTimeSyncContext.Provider value={readonlyReactTs}>
			{children}
		</reactTimeSyncContext.Provider>
	);
};

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
	 * callback instead of the main Date state.
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
	const activeTransform = (transform ?? identity) as TransformCallback<
		T & Date
	>;

	const hookId = useId();
	const reactTs = useReactTimeSync();

	// Because of how React lifecycles work, the effect event callback is
	// *never* safe to call from inside render logic. It will *always* give you
	// stale data after the very first render.
	const externalTransform = useEffectEvent(activeTransform);
	const subscribe: ReactSubscriptionCallback = useCallback(
		(notifyReact) => {
			return reactTs.subscribe({
				componentId: hookId,
				targetRefreshIntervalMs: targetIntervalMs,
				transform: externalTransform,
				onStateUpdate: notifyReact,
			});
		},

		// All dependencies listed for correctness, but targetInterval is the
		// only value that can change on re-renders
		[reactTs, hookId, externalTransform, targetIntervalMs],
	);

	const dateState = useSyncExternalStore(subscribe, reactTs.getDateSnapshot);

	// There's some trade-offs with this setup (notably, if the consumer passes
	// in an inline function, the memo result will be invalidated on every
	// single render). But it's the *only* way to give the consumer the option
	// of memoizing expensive transformations at the render level without
	// polluting the hook's API with super-fragile dependency array nonsense
	const transformed = useMemo<T>(
		() => activeTransform(dateState),
		[activeTransform, dateState],
	);

	useEffect(() => {
		reactTs.updateCachedTransformation(hookId, transformed);
	}, [reactTs, hookId, transformed]);

	return transformed;
}
