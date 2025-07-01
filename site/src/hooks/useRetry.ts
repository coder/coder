import { useCallback, useEffect, useReducer, useRef } from "react";
import { useEffectEvent } from "./hookPolyfills";

interface UseRetryOptions {
	/**
	 * Function to call when retrying
	 */
	onRetry: () => Promise<void>;
	/**
	 * Maximum number of retry attempts
	 */
	maxAttempts: number;
	/**
	 * Initial delay in milliseconds
	 */
	initialDelay: number;
	/**
	 * Maximum delay in milliseconds
	 */
	maxDelay: number;
	/**
	 * Backoff multiplier
	 */
	multiplier: number;
	/**
	 * Whether retry is enabled
	 */
	enabled: boolean;
}

interface UseRetryReturn {
	/**
	 * Manually trigger a retry
	 */
	retry: () => void;
	/**
	 * Whether a retry is currently in progress (manual or automatic)
	 */
	isRetrying: boolean;
	/**
	 * Current delay for the next automatic retry (null if not scheduled)
	 */
	currentDelay: number | null;
	/**
	 * Number of retry attempts made
	 */
	attemptCount: number;
	/**
	 * Time in milliseconds until the next automatic retry (null if not scheduled)
	 */
	timeUntilNextRetry: number | null;
}

interface RetryState {
	isRetrying: boolean;
	currentDelay: number | null;
	attemptCount: number;
	timeUntilNextRetry: number | null;
	isManualRetry: boolean;
}

type RetryAction =
	| { type: "START_RETRY" }
	| { type: "RETRY_SUCCESS" }
	| { type: "RETRY_FAILURE" }
	| { type: "SCHEDULE_RETRY"; delay: number }
	| { type: "UPDATE_COUNTDOWN"; timeRemaining: number }
	| { type: "CANCEL_RETRY" }
	| { type: "RESET" }
	| { type: "SET_MANUAL_RETRY"; isManual: boolean };

const initialState: RetryState = {
	isRetrying: false,
	currentDelay: null,
	attemptCount: 0,
	timeUntilNextRetry: null,
	isManualRetry: false,
};

function retryReducer(state: RetryState, action: RetryAction): RetryState {
	switch (action.type) {
		case "START_RETRY":
			return {
				...state,
				isRetrying: true,
				currentDelay: null,
				timeUntilNextRetry: null,
				attemptCount: state.attemptCount + 1,
			};
		case "RETRY_SUCCESS":
			return {
				...initialState,
			};
		case "RETRY_FAILURE":
			return {
				...state,
				isRetrying: false,
				isManualRetry: false,
			};
		case "SCHEDULE_RETRY":
			return {
				...state,
				currentDelay: action.delay,
				timeUntilNextRetry: action.delay,
			};
		case "UPDATE_COUNTDOWN":
			return {
				...state,
				timeUntilNextRetry: action.timeRemaining,
			};
		case "CANCEL_RETRY":
			return {
				...state,
				currentDelay: null,
				timeUntilNextRetry: null,
			};
		case "RESET":
			return {
				...initialState,
			};
		case "SET_MANUAL_RETRY":
			return {
				...state,
				isManualRetry: action.isManual,
			};
		default:
			return state;
	}
}

/**
 * Hook for handling exponential backoff retry logic
 */
export function useRetry(options: UseRetryOptions): UseRetryReturn {
	const { onRetry, maxAttempts, initialDelay, maxDelay, multiplier, enabled } =
		options;
	const [state, dispatch] = useReducer(retryReducer, initialState);

	const timeoutRef = useRef<number | null>(null);
	const countdownRef = useRef<number | null>(null);
	const startTimeRef = useRef<number | null>(null);
	const hasStartedRef = useRef<boolean>(false);

	const onRetryEvent = useEffectEvent(onRetry);

	const clearTimers = useCallback(() => {
		if (timeoutRef.current) {
			window.clearTimeout(timeoutRef.current);
			timeoutRef.current = null;
		}
		if (countdownRef.current) {
			window.clearInterval(countdownRef.current);
			countdownRef.current = null;
		}
		startTimeRef.current = null;
	}, []);

	const calculateDelay = useCallback(
		(attempt: number): number => {
			const delay = initialDelay * multiplier ** attempt;
			return Math.min(delay, maxDelay);
		},
		[initialDelay, multiplier, maxDelay],
	);

	// biome-ignore lint/correctness/useExhaustiveDependencies: onRetryEvent is created with useEffectEvent and is stable
	const performRetry = useCallback(async () => {
		dispatch({ type: "START_RETRY" });
		clearTimers();

		try {
			await onRetryEvent();
			// If retry succeeds, reset everything
			dispatch({ type: "RETRY_SUCCESS" });
		} catch (error) {
			// If retry fails, just update state
			dispatch({ type: "RETRY_FAILURE" });
		}
	}, [clearTimers]);

	const scheduleNextRetry = useCallback(
		(attempt: number) => {
			if (attempt > maxAttempts) {
				return;
			}

			// Calculate delay based on attempt - 1 (so first retry gets initialDelay)
			const delay = calculateDelay(Math.max(0, attempt - 1));
			dispatch({ type: "SCHEDULE_RETRY", delay });
			startTimeRef.current = Date.now();

			// Start countdown timer
			countdownRef.current = window.setInterval(() => {
				if (startTimeRef.current) {
					const elapsed = Date.now() - startTimeRef.current;
					const remaining = Math.max(0, delay - elapsed);
					dispatch({ type: "UPDATE_COUNTDOWN", timeRemaining: remaining });

					if (remaining <= 0) {
						if (countdownRef.current) {
							window.clearInterval(countdownRef.current);
							countdownRef.current = null;
						}
					}
				}
			}, 100); // Update every 100ms for smooth countdown

			// Schedule the actual retry
			timeoutRef.current = window.setTimeout(() => {
				performRetry();
			}, delay);
		},
		[calculateDelay, maxAttempts, performRetry],
	);

	// Effect to handle enabled state and retry scheduling
	useEffect(() => {
		if (!enabled) {
			// When disabled, clear timers and reset state
			clearTimers();
			dispatch({ type: "RESET" });
			hasStartedRef.current = false;
			return;
		}

		// When enabled and no attempts yet, start first retry (only once)
		if (
			enabled &&
			state.attemptCount === 0 &&
			!state.isRetrying &&
			!hasStartedRef.current
		) {
			hasStartedRef.current = true;
			performRetry();
			return;
		}

		// Schedule next retry after a failed attempt
		if (
			enabled &&
			!state.isRetrying &&
			!state.isManualRetry &&
			state.attemptCount > 0 &&
			state.attemptCount < maxAttempts
		) {
			scheduleNextRetry(state.attemptCount);
		}
	}, [
		enabled,
		state.attemptCount,
		state.isRetrying,
		state.isManualRetry,
		maxAttempts,
		clearTimers,
		performRetry,
		scheduleNextRetry,
	]);

	const retry = useCallback(() => {
		if (!enabled) return; // Don't allow manual retry when disabled
		dispatch({ type: "SET_MANUAL_RETRY", isManual: true });
		clearTimers();
		dispatch({ type: "CANCEL_RETRY" });
		performRetry();
	}, [enabled, clearTimers, performRetry]);

	// Cleanup on unmount
	useEffect(() => {
		return () => {
			clearTimers();
		};
	}, [clearTimers]);

	return {
		retry,
		isRetrying: state.isRetrying,
		currentDelay: state.currentDelay,
		attemptCount: state.attemptCount,
		timeUntilNextRetry: state.timeUntilNextRetry,
	};
}
