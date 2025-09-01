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
	type ReadonlyDate,
	TimeSync,
	defaultOptions as timeSyncDefaultOptions,
} from "utils/TimeSync";
import { useEffectEvent } from "./hookPolyfills";

type SubscriptionCallback = (notifyReact: () => void) => () => void;

const REFRESH_ONE_SECOND = 1_000;
const REFRESH_ONE_MINUTE = 60 * 1_000;
const REFRESH_ONE_HOUR = 60 * 60 * 1_000;
const REFRESH_ONE_DAY = 24 * 60 * 60 * 1_000;

// Combines two pieces of state while trying to maintain as much structural
// sharing as possible
function mergeSnapshots(oldValue: unknown, newValue: unknown): unknown {
	if (oldValue === newValue) {
		return oldValue;
	}
	return newValue;
}

type ReactTimeSyncInitOptions = Readonly<{
	initialDatetime: Date;
	minimumRefreshIntervalMs: number;
	disableUpdates: boolean;
}>;

type TransformCallback<T> = (
	state: ReadonlyDate,
) => T extends Promise<unknown> ? never : T;

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

	getTimeSync(): TimeSync {
		return this.#timeSync;
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

		let onUpdate: OnUpdate = noOp;
		if (!this.#disableUpdates) {
			onUpdate = (newDate) => {
				const entry = this.#entries.get(componentId);
				if (entry === undefined) {
					return;
				}

				const newState = transform(newDate);
				const merged = mergeSnapshots(entry.lastTransformedValue, newState);

				if (entry.lastTransformedValue !== merged) {
					entry.lastTransformedValue = merged;
					onStateUpdate();
				}
			};
		}

		// Even if updates are disabled, we still need to set up a subscription,
		// so that we satisfy the API for useSyncExternalStore. So if TimeSync
		// is disabled, we'll just pass in a no-op function instead
		const unsubscribe = this.#timeSync.subscribe({
			targetRefreshIntervalMs,
			onUpdate,
		});

		const latestSyncState = this.#timeSync.getStateSnapshot();
		const newEntry: SubscriptionEntry = {
			unsubscribe,
			/**
			 * @todo 2025-08-30 - There is one unfortunate behavior with the
			 * current subscription logic. Because of how React lifecycles work,
			 * each new component instance needs to call the transform callback
			 * twice on setup. You need to call it once from the render, and
			 * again from the subscribe method.
			 *
			 * Trying to fix this got really nasty (even with a bunch of cursed
			 * React techniques), and caused a bunch of chicken-and-the-egg
			 * problems. Luckily, most transformations should be super cheap,
			 * which should buy us some time to get this fixed.
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
		const updated = mergeSnapshots(entry.lastTransformedValue, newValue);
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
	getDateSnapshot = (): ReadonlyDate => {
		return this.#timeSync.getStateSnapshot();
	};
}

const timeSyncContext = createContext<ReactTimeSync | null>(null);

function useReactTimeSync(): ReactTimeSync {
	const reactTs = useContext(timeSyncContext);
	if (reactTs === null) {
		throw new Error(
			`Must call TimeSync hook from inside ${TimeSyncProvider.name}`,
		);
	}
	return reactTs;
}

function identity<T>(value: T): T {
	return value;
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
		<timeSyncContext.Provider value={readonlyReactTs}>
			{children}
		</timeSyncContext.Provider>
	);
};

/**
 * Exposes the TimeSync instance currently being dependency-injected throughout
 * the application. This lets you set up manual subscriptions for effect logic.
 *
 * Most of the time, you should not need this, especially if you need data from
 * TimeSync to be exposed for render logic. Consider using `useTimeSyncState`
 * instead.
 */
export function useTimeSync(): TimeSync {
	const wrapper = useReactTimeSync();
	return wrapper.getTimeSync();
}

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
	 *    `useCallback` got invalidated.
	 * 3. When TimeSync dispatches a new date update, it will run the latest
	 *    `transform` callback. If the result has not changed same (comparing by
	 *    value), the subscribed component will not re-render by itself.
	 *
	 * `transform` callbacks must not be async. The hook will error out at the
	 * type level if you provide one by mistake.
	 */
	transform?: TransformCallback<T>;
}>;

export function useTimeSyncState<T = ReadonlyDate>(
	options: UseTimeSyncOptions<T>,
): T {
	const { targetIntervalMs, transform } = options;
	const activeTransform = (transform ?? identity) as TransformCallback<
		T & ReadonlyDate
	>;

	const hookId = useId();
	const reactTs = useReactTimeSync();

	// Because of how React lifecycles work, the effect event callback is
	// *never* safe to call from inside render logic. It will *always* give you
	// stale data after the very first render.
	const externalTransform = useEffectEvent(activeTransform);
	const subscribe: SubscriptionCallback = useCallback(
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
