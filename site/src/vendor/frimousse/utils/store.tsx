// Vendored from frimousse 0.4.0 (MIT; see LICENSE in this directory).
// Upstream: https://github.com/liveblocks/frimousse/tree/5723fc1/src
// Local deviations must be marked LOCAL CHANGE.

import {
  createContext,
  type PropsWithChildren,
  useCallback,
  useContext,
  useDebugValue,
  useEffect,
  useState,
} from "react";

// A tiny store with batched updates, context support, and selectors.

export type Store<T> = {
  get: () => T;
  set: (partial: Partial<T> | ((state: T) => Partial<T>)) => void;
  subscribe: (subscriber: (state: T) => void) => () => void;
};

export function createStore<T extends object>(
  createInitialState: (set: Store<T>["set"], get: Store<T>["get"]) => T,
): Store<T> {
  let state = {} as T;
  let pending: T | null = null;
  let frameId: number | null = null;
  const subscribers = new Set<(store: T) => void>();

  const flush = () => {
    if (pending) {
      state = pending;
      pending = null;

      for (const subscriber of subscribers) {
        subscriber(state);
      }
    }

    frameId = null;
  };

  const get = () => pending ?? state;

  const set: Store<T>["set"] = (partial) => {
    pending ??= state;
    Object.assign(
      pending as T,
      typeof partial === "function"
        ? (partial as (state: T) => Partial<T>)(get())
        : partial,
    );

    if (!frameId) {
      frameId = requestAnimationFrame(flush);
    }
  };

  const subscribe = (subscriber: (state: T) => void) => {
    subscribers.add(subscriber);

    return () => subscribers.delete(subscriber);
  };

  state = createInitialState(set, get);

  return { get, set, subscribe };
}

export function useCreateStore<T>(createStore: () => Store<T>) {
  const [store] = useState(createStore);

  return store;
}

export function createStoreContext<T>(missingProviderError?: string) {
  const Context = createContext<Store<T> | null>(null);

  const useStore = () => {
    const store = useContext(Context);

    if (!store) {
      throw new Error(missingProviderError);
    }

    return store as Store<T>;
  };

  const Provider = ({
    store,
    children,
  }: PropsWithChildren<{ store: Store<T> }>) => {
    return <Context.Provider value={store}>{children}</Context.Provider>;
  };

  return { useStore, Provider };
}

export function useSelector<T, S>(
  store: Store<T>,
  selector: (state: T) => S,
  compare: (a: S, b: S) => boolean = Object.is,
) {
  const [slice, setSlice] = useState(() => selector(store.get()));

  useEffect(() => {
    return store.subscribe(() => {
      const nextSlice = selector(store.get());

      setSlice((previousSlice) =>
        compare(previousSlice, nextSlice) ? previousSlice : nextSlice,
      );
    });
  }, [store, selector, compare]);

  useDebugValue(slice);

  return slice;
}

export function useSelectorKey<T, K extends keyof T>(
  store: Store<T>,
  key: K,
  compare?: (a: T[K], b: T[K]) => boolean,
) {
  const selector = useCallback((state: T) => state[key], [key]);

  return useSelector(store, selector, compare);
}
