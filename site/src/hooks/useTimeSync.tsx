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

type ReactSubscriptionCallback = (notifyReact: () => void) => () => void;

type SubscriptionEntry = Readonly<{
  id: string;
  maximumRefreshIntervalMs: number;
  onIntervalTick: (newDatetime: Date) => void;
}>;

type SetInterval = (fn: () => void, intervalMs: number) => number;
type ClearInterval = (id: number | undefined) => void;

type TimeSyncInitOptions = Readonly<
  Partial<{
    initialDatetime: Date;
    setInterval: SetInterval;
    clearInterval: ClearInterval;
  }>
>;

const defaultOptions: Required<TimeSyncInitOptions> = {
  initialDatetime: new Date(),
  setInterval: window.setInterval,
  clearInterval: window.clearInterval,
};

interface TimeSyncApi {
  getCurrentDatetime: () => Date;
  subscribe: (entry: SubscriptionEntry) => () => void;
  unsubscribe: (id: string) => void;
}

export class TimeSync implements TimeSyncApi {
  #currentDatetime: Date;
  #subscriptions: SubscriptionEntry[];
  #latestIntervalId: number | undefined;
  readonly #setInterval: SetInterval;
  readonly #clearInterval: ClearInterval;

  constructor(options: TimeSyncInitOptions) {
    const {
      initialDatetime = defaultOptions.initialDatetime,
      setInterval = defaultOptions.setInterval,
      clearInterval = defaultOptions.clearInterval,
    } = options;

    this.#currentDatetime = initialDatetime;
    this.#subscriptions = [];
    this.#latestIntervalId = undefined;
    this.#setInterval = setInterval;
    this.#clearInterval = clearInterval;
  }

  #reconcileRefreshIntervals(): void {
    this.#subscriptions.sort(
      (e1, e2) => e1.maximumRefreshIntervalMs - e2.maximumRefreshIntervalMs,
    );
    this.#notifySubscriptions();
  }

  #notifySubscriptions(): void {
    for (const subEntry of this.#subscriptions) {
      subEntry.onIntervalTick(this.#currentDatetime);
    }
  }

  // All functions that are part of the public interface must be defined as
  // arrow functions, so that they work properly with React

  getCurrentDatetime = (): Date => {
    return this.#currentDatetime;
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

function identity<T>(value: T): T {
  return value;
}

type UseTimeSyncOptions<T = Date> = Readonly<{
  maximumRefreshIntervalMs?: number;
  select?: (newDate: Date) => T;
}>;

export function useTimeSync<T = Date>(options?: UseTimeSyncOptions<T>): T {
  const { select = identity, maximumRefreshIntervalMs = Infinity } =
    options ?? {};

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
        maximumRefreshIntervalMs,
        onIntervalTick: notifyReact,
        id: hookId,
      });
    },
    [timeSync, hookId, maximumRefreshIntervalMs],
  );

  const currentTime = useSyncExternalStore(subscribe, () =>
    timeSync.getCurrentDatetime(),
  );

  return select(currentTime as T & Date);
}
