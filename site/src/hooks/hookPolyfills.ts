/**
 * @file For defining DIY versions of official React hooks that have not been
 * released yet.
 *
 * These hooks should be deleted as soon as the official versions are available.
 * They do not have the same ESLinter exceptions baked in that the official
 * hooks do, especially for dependency arrays.
 */
import { useCallback, useLayoutEffect, useRef } from "react";

/**
 * A DIY version of useEffectEvent.
 *
 * Works like useCallback, except that it doesn't take a dependency array, and
 * always returns out the same function on every single render. The returned-out
 * function is always able to "see" the most up-to-date version of the callback
 * passed in (including its closure values).
 *
 * This is not a 1:1 replacement for useCallback. 99% of the time,
 * useEffectEvent should be called in the same component/custom hook where you
 * have a useEffect call. A useEffectEvent function probably shouldn't be a
 * prop, unless you're trying to wrangle a weird library.
 *
 * Example uses of useEffectEvent:
 * 1. Stabilizing a function that you don't have direct control over (because it
 *    comes from a library) without violating useEffect dependency arrays
 * 2. Moving the burden of memoization from the parent to the custom hook (e.g.,
 *    making it so that you don't need your components to always use useCallback
 *    just to get things wired up properly. Similar example: the queryFn
 *    property on React Query's useQuery)
 *
 * @see {@link https://react.dev/reference/react/experimental_useEffectEvent}
 */
export function useEffectEvent<TArgs extends unknown[], TReturn = unknown>(
  callback: (...args: TArgs) => TReturn,
) {
  const callbackRef = useRef(callback);

  // useLayoutEffect should be overkill here 99% of the time, but if this were
  // defined as a regular effect, useEffectEvent would not be able to work with
  // any layout effects at all; the callback sync here would fire *after* the
  // layout effect that needs the useEffectEvent function
  useLayoutEffect(() => {
    callbackRef.current = callback;
  }, [callback]);

  return useCallback((...args: TArgs): TReturn => {
    return callbackRef.current(...args);
  }, []);
}
