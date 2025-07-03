import { useCallback, useEffect, useRef, useState } from "react";
import { useEffectEvent } from "./hookPolyfills";

const DELAY_MS = 1_000;
const MAX_DELAY_MS = 600_000; // 10 minutes
// Determines how much the delay between retry attempts increases after each
// failure.
const MULTIPLIER = 2;

interface UseWithRetryResult {
	call: () => void;
	nextRetryAt: Date | undefined;
	isLoading: boolean;
}

interface RetryState {
	isLoading: boolean;
	nextRetryAt: Date | undefined;
}

/**
 * Hook that wraps a function with automatic retry functionality
 * Provides a simple interface for executing functions with exponential backoff retry
 */
export function useWithRetry(fn: () => Promise<void>): UseWithRetryResult {
	const [state, setState] = useState<RetryState>({
		isLoading: false,
		nextRetryAt: undefined,
	});

	const timeoutRef = useRef<number | null>(null);
	const mountedRef = useRef(true);

	const clearTimeout = useCallback(() => {
		if (timeoutRef.current) {
			window.clearTimeout(timeoutRef.current);
			timeoutRef.current = null;
		}
	}, []);

	const stableFn = useEffectEvent(fn);

	const call = useCallback(() => {
		if (state.isLoading) {
			return;
		}

		clearTimeout();

		const executeAttempt = async (attempt = 0): Promise<void> => {
			if (!mountedRef.current) {
				return;
			}
			setState({
				isLoading: true,
				nextRetryAt: undefined,
			});

			try {
				await stableFn();
				if (mountedRef.current) {
					setState({ isLoading: false, nextRetryAt: undefined });
				}
			} catch (error) {
				if (!mountedRef.current) {
					return;
				}
				const delayMs = Math.min(
					DELAY_MS * MULTIPLIER ** attempt,
					MAX_DELAY_MS,
				);

				setState({
					isLoading: false,
					nextRetryAt: new Date(Date.now() + delayMs),
				});

				timeoutRef.current = window.setTimeout(() => {
					if (!mountedRef.current) {
						return;
					}
					setState({
						isLoading: false,
						nextRetryAt: undefined,
					});
					executeAttempt(attempt + 1);
				}, delayMs);
			}
		};

		executeAttempt();
	}, [state.isLoading, stableFn, clearTimeout]);

	useEffect(() => {
		return () => {
			mountedRef.current = false;
			clearTimeout();
		};
	}, [clearTimeout]);

	return {
		call,
		nextRetryAt: state.nextRetryAt,
		isLoading: state.isLoading,
	};
}
