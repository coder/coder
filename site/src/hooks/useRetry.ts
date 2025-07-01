import { useCallback, useEffect, useRef, useState } from "react";
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
	/**
	 * Start the retry process
	 */
	startRetrying: () => void;
	/**
	 * Stop the retry process and reset state
	 */
	stopRetrying: () => void;
}

/**
 * Hook for handling exponential backoff retry logic
 */
export function useRetry(options: UseRetryOptions): UseRetryReturn {
	const { onRetry, maxAttempts, initialDelay, maxDelay, multiplier } = options;
	const [isRetrying, setIsRetrying] = useState(false);
	const [currentDelay, setCurrentDelay] = useState<number | null>(null);
	const [attemptCount, setAttemptCount] = useState(0);
	const [timeUntilNextRetry, setTimeUntilNextRetry] = useState<number | null>(
		null,
	);
	const [isManualRetry, setIsManualRetry] = useState(false);

	const timeoutRef = useRef<number | null>(null);
	const countdownRef = useRef<number | null>(null);
	const startTimeRef = useRef<number | null>(null);

	const onRetryEvent = useEffectEvent(onRetry);

	const clearTimers = useCallback(() => {
		if (timeoutRef.current) {
			clearTimeout(timeoutRef.current);
			timeoutRef.current = null;
		}
		if (countdownRef.current) {
			clearInterval(countdownRef.current);
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

	const performRetry = useCallback(async () => {
		setIsRetrying(true);
		setTimeUntilNextRetry(null);
		setCurrentDelay(null);
		clearTimers();
		// Increment attempt count when starting the retry
		setAttemptCount(prev => prev + 1);

		try {
			await onRetryEvent();
			// If retry succeeds, reset everything
			setAttemptCount(0);
			setIsRetrying(false);
			setIsManualRetry(false);
		} catch (error) {
			// If retry fails, just update state (attemptCount already incremented)
			setIsRetrying(false);
			setIsManualRetry(false);
		}
	}, [onRetryEvent, clearTimers]);

	const scheduleNextRetry = useCallback(
		(attempt: number) => {
			if (attempt > maxAttempts) {
				return;
			}

			// Calculate delay based on attempt - 1 (so first retry gets initialDelay)
		const delay = calculateDelay(Math.max(0, attempt - 1));
			setCurrentDelay(delay);
			setTimeUntilNextRetry(delay);
			startTimeRef.current = Date.now();

			// Start countdown timer
			countdownRef.current = setInterval(() => {
				if (startTimeRef.current) {
					const elapsed = Date.now() - startTimeRef.current;
					const remaining = Math.max(0, delay - elapsed);
					setTimeUntilNextRetry(remaining);

					if (remaining <= 0) {
						if (countdownRef.current) {
							clearInterval(countdownRef.current);
							countdownRef.current = null;
						}
					}
				}
			}, 100); // Update every 100ms for smooth countdown

			// Schedule the actual retry
			timeoutRef.current = setTimeout(() => {
				performRetry();
			}, delay);
		},
		[calculateDelay, maxAttempts, performRetry],
	);

	// Effect to schedule next retry after a failed attempt
	useEffect(() => {
		if (
			!isRetrying &&
			!isManualRetry &&
			attemptCount > 0 &&
			attemptCount < maxAttempts
		) {
			scheduleNextRetry(attemptCount);
		}
	}, [attemptCount, isRetrying, isManualRetry, maxAttempts, scheduleNextRetry]);

	const retry = useCallback(() => {
		setIsManualRetry(true);
		clearTimers();
		setTimeUntilNextRetry(null);
		setCurrentDelay(null);
		performRetry();
	}, [clearTimers, performRetry]);

	const startRetrying = useCallback(() => {
		// Immediately perform the first retry attempt
		performRetry();
	}, [performRetry]);

	const stopRetrying = useCallback(() => {
		clearTimers();
		setIsRetrying(false);
		setCurrentDelay(null);
		setAttemptCount(0);
		setTimeUntilNextRetry(null);
		setIsManualRetry(false);
	}, [clearTimers]);

	// Cleanup on unmount
	useEffect(() => {
		return () => {
			clearTimers();
		};
	}, [clearTimers]);

	return {
		retry,
		isRetrying,
		currentDelay,
		attemptCount,
		timeUntilNextRetry,
		startRetrying,
		stopRetrying,
	};
}
