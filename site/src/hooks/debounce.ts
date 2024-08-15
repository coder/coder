/**
 * @file Defines hooks for created debounced versions of functions and arbitrary
 * values.
 *
 * It is not safe to call a general-purpose debounce utility inside a React
 * render. It will work on the initial render, but the memory reference for the
 * value will change on re-renders. Most debounce functions create a "stateful"
 * version of a function by leveraging closure; but by calling it repeatedly,
 * you create multiple "pockets" of state, rather than a centralized one.
 *
 * Debounce utilities can make sense if they can be called directly outside the
 * component or in a useEffect call, though.
 */
import { useCallback, useEffect, useRef, useState } from "react";

type useDebouncedFunctionReturn<Args extends unknown[]> = Readonly<{
	debounced: (...args: Args) => void;

	// Mainly here to make interfacing with useEffect cleanup functions easier
	cancelDebounce: () => void;
}>;

/**
 * Creates a debounce function that is resilient to React re-renders, as well as
 * a function for canceling a pending debounce.
 *
 * The returned-out functions will maintain the same memory references, but the
 * debounce function will always "see" the most recent versions of the arguments
 * passed into the hook, and use them accordingly.
 *
 * If the debounce time changes while a callback has been queued to fire, the
 * callback will be canceled completely. You will need to restart the debounce
 * process by calling the returned-out function again.
 */
export function useDebouncedFunction<
	// Parameterizing on the args instead of the whole callback function type to
	// avoid type contra-variance issues
	Args extends unknown[] = unknown[],
>(
	callback: (...args: Args) => void | Promise<void>,
	debounceTimeMs: number,
): useDebouncedFunctionReturn<Args> {
	const timeoutIdRef = useRef<number | null>(null);
	const cancelDebounce = useCallback(() => {
		if (timeoutIdRef.current !== null) {
			window.clearTimeout(timeoutIdRef.current);
		}

		timeoutIdRef.current = null;
	}, []);

	const debounceTimeRef = useRef(debounceTimeMs);
	useEffect(() => {
		cancelDebounce();
		debounceTimeRef.current = debounceTimeMs;
	}, [cancelDebounce, debounceTimeMs]);

	const callbackRef = useRef(callback);
	useEffect(() => {
		callbackRef.current = callback;
	}, [callback]);

	// Returned-out function will always be synchronous, even if the callback arg
	// is async. Seemed dicey to try awaiting a genericized operation that can and
	// will likely be canceled repeatedly
	const debounced = useCallback(
		(...args: Args): void => {
			cancelDebounce();

			timeoutIdRef.current = window.setTimeout(
				() => void callbackRef.current(...args),
				debounceTimeRef.current,
			);
		},
		[cancelDebounce],
	);

	return { debounced, cancelDebounce } as const;
}

/**
 * Takes any value, and returns out a debounced version of it.
 */
export function useDebouncedValue<T = unknown>(
	value: T,
	debounceTimeMs: number,
): T {
	const [debouncedValue, setDebouncedValue] = useState(value);

	useEffect(() => {
		const timeoutId = window.setTimeout(() => {
			setDebouncedValue(value);
		}, debounceTimeMs);

		return () => window.clearTimeout(timeoutId);
	}, [value, debounceTimeMs]);

	return debouncedValue;
}
