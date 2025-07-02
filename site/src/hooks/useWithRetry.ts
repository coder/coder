import { useCallback, useEffect, useRef, useState } from "react";

const MAX_ATTEMPTS = 10;
const DELAY_MS = 1_000;
const MAX_DELAY_MS = 600_000; // 10 minutes
// Determines how much the delay between retry attempts increases after each
// failure.
const MULTIPLIER = 2;

interface UseWithRetryResult {
	call: () => Promise<void>;
	retryAt: Date | undefined;
	isLoading: boolean;
}

interface RetryState {
	isLoading: boolean;
	retryAt: Date | undefined;
	attemptCount: number;
}

/**
 * Hook that wraps a function with automatic retry functionality
 * Provides a simple interface for executing functions with exponential backoff retry
 */
export function useWithRetry(fn: () => Promise<void>): UseWithRetryResult {
	const [state, setState] = useState<RetryState>({
		isLoading: false,
		retryAt: undefined,
		attemptCount: 0,
	});

	const timeoutRef = useRef<number | null>(null);

	const clearTimeout = useCallback(() => {
		if (timeoutRef.current) {
			window.clearTimeout(timeoutRef.current);
			timeoutRef.current = null;
		}
	}, []);

	const call = useCallback(async () => {
		clearTimeout();

		const executeAttempt = async (attempt: number): Promise<void> => {
			setState((prev) => ({ ...prev, isLoading: true, attemptCount: attempt }));

			try {
				await fn();
				setState({ isLoading: false, retryAt: undefined, attemptCount: 0 });
			} catch (error) {
				// Since attempts start from 0, we need to add +1 to make the condition work
				// This ensures exactly MAX_ATTEMPTS total attempts (attempt 0, 1, 2, ..., 9)
				if (attempt + 1 < MAX_ATTEMPTS) {
					const delayMs = Math.min(
						DELAY_MS * MULTIPLIER ** attempt,
						MAX_DELAY_MS,
					);

					setState((prev) => ({
						...prev,
						isLoading: false,
						retryAt: new Date(Date.now() + delayMs),
					}));

					timeoutRef.current = window.setTimeout(() => {
						setState((prev) => ({ ...prev, retryAt: undefined }));
						executeAttempt(attempt + 1);
					}, delayMs);
				} else {
					setState((prev) => ({
						...prev,
						isLoading: false,
						retryAt: undefined,
					}));
				}
			}
		};

		await executeAttempt(0);
	}, [fn, clearTimeout]);

	useEffect(() => {
		return () => {
			clearTimeout();
		};
	}, [clearTimeout]);

	return {
		call,
		retryAt: state.retryAt,
		isLoading: state.isLoading,
	};
}
