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

export const MAX_REFRESH_ONE_MINUTE = 60 * 1_000;
export const MAX_REFRESH_ONE_DAY = 24 * 60 * 60 * 1_000;

type SetInterval = (fn: () => void, intervalMs: number) => number;
type ClearInterval = (id: number | undefined) => void;

type TimeSyncInitOptions = Readonly<
	Partial<{
		initialDatetime: Date;
		createNewDateime: () => Date;
		setInterval: SetInterval;
		clearInterval: ClearInterval;
	}>
>;

const defaultOptions: Required<TimeSyncInitOptions> = {
	initialDatetime: new Date(),
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

export class TimeSync implements TimeSyncApi {
	readonly #createNewDatetime: () => Date;
	readonly #setInterval: SetInterval;
	readonly #clearInterval: ClearInterval;

	#latestSnapshot: Date;
	#subscriptions: SubscriptionEntry[];
	#latestIntervalId: number | undefined;

	constructor(options: TimeSyncInitOptions) {
		const {
			initialDatetime = defaultOptions.initialDatetime,
			createNewDateime = defaultOptions.createNewDateime,
			setInterval = defaultOptions.setInterval,
			clearInterval = defaultOptions.clearInterval,
		} = options;

		this.#latestSnapshot = initialDatetime;
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
			this.#latestSnapshot = this.#createNewDatetime();
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

		this.#subscriptions.push(entry);
		this.#reconcileRefreshIntervals();
		return () => this.unsubscribe(entry.id);
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
		options?: TimeSyncInitOptions;
	}>
>;

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
	select?: (newDate: Date) => T;
}>;

type ReactSubscriptionCallback = (notifyReact: () => void) => () => void;

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
