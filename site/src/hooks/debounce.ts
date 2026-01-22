/**
 * @file Defines hooks for created debounced versions of functions and arbitrary
 * values.
 *
 * It is not safe to call most general-purpose debounce utility functions inside
 * a React render. This is because the state for handling the debounce logic
 * lives in the utility instead of React. If you call a general-purpose debounce
 * function inline, that will create a new stateful function on every render,
 * which has a lot of risks around conflicting/contradictory state.
 */
import { useCallback, useEffect, useRef, useState } from "react";

type UseDebouncedFunctionReturn<Args extends unknown[]> = Readonly<{
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
	// avoid type contravariance issues
	Args extends unknown[] = unknown[],
>(
	callback: (...args: Args) => void | Promise<void>,
	debounceTimeoutMs: number,
): UseDebouncedFunctionReturn<Args> {
	if (!Number.isInteger(debounceTimeoutMs) || debounceTimeoutMs < 0) {
		throw new Error(
			`Invalid value ${debounceTimeoutMs} for debounceTimeoutMs. Value must be an integer greater than or equal to zero.`,
		);
	}

	const timeoutIdRef = useRef<number | undefined>(undefined);
	const cancelDebounce = useCallback(() => {
		if (timeoutIdRef.current !== undefined) {
			clearTimeout(timeoutIdRef.current);
		}

		timeoutIdRef.current = undefined;
	}, []);

	const debounceTimeRef = useRef(debounceTimeoutMs);
	useEffect(() => {
		cancelDebounce();
		debounceTimeRef.current = debounceTimeoutMs;
	}, [cancelDebounce, debounceTimeoutMs]);

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

			timeoutIdRef.current = setTimeout(
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
export function useDebouncedValue<T>(value: T, debounceTimeoutMs: number): T {
	if (!Number.isInteger(debounceTimeoutMs) || debounceTimeoutMs < 0) {
		throw new Error(
			`Invalid value ${debounceTimeoutMs} for debounceTimeoutMs. Value must be an integer greater than or equal to zero.`,
		);
	}

	const [debouncedValue, setDebouncedValue] = useState(value);

	// If the debounce timeout is ever zero, synchronously flush any state syncs.
	// Doing this mid-render instead of in useEffect means that we drastically cut
	// down on needless re-renders, and we also avoid going through the event loop
	// to do a state sync that is *intended* to happen immediately
	if (value !== debouncedValue && debounceTimeoutMs === 0) {
		setDebouncedValue(value);
	}
	useEffect(() => {
		if (debounceTimeoutMs === 0) {
			return;
		}

		const timeoutId = setTimeout(() => {
			setDebouncedValue(value);
		}, debounceTimeoutMs);
		return () => clearTimeout(timeoutId);
	}, [value, debounceTimeoutMs]);

	return debouncedValue;
}
