import { useCallback, useEffect, useRef, useState } from "react";

// Configuration constants
const MAX_ATTEMPTS = 10;
const INITIAL_DELAY = 1000; // 1 second
const MAX_DELAY = 600000; // 10 minutes
const MULTIPLIER = 2;

interface UseWithRetryResult {
  call: () => Promise<void>;
  retryAt: Date | null;
  isLoading: boolean;
  attemptCount: number;
}

interface RetryState {
  isLoading: boolean;
  retryAt: Date | null;
  attemptCount: number;
}

/**
 * Hook that wraps a function with automatic retry functionality
 * Provides a simple interface for executing functions with exponential backoff retry
 */
export function useWithRetry(fn: () => Promise<void>): UseWithRetryResult {
  const [state, setState] = useState<RetryState>({
    isLoading: false,
    retryAt: null,
    attemptCount: 0,
  });

  const timeoutRef = useRef<number | null>(null);

  const clearTimer = useCallback(() => {
    if (timeoutRef.current) {
      window.clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
  }, []);

  const call = useCallback(async () => {
    // Cancel any existing retry and start fresh
    clearTimer();
    
    const executeAttempt = async (attempt: number): Promise<void> => {
      setState(prev => ({ ...prev, isLoading: true, attemptCount: attempt }));

      try {
        await fn();
        // Success - reset everything
        setState({ isLoading: false, retryAt: null, attemptCount: 0 });
      } catch (error) {
        // Failure - schedule retry if attempts remaining
        if (attempt < MAX_ATTEMPTS) {
          const delay = Math.min(INITIAL_DELAY * MULTIPLIER ** attempt, MAX_DELAY);
          const retryTime = new Date(Date.now() + delay);
          
          setState(prev => ({ ...prev, isLoading: false, retryAt: retryTime }));

          // Schedule the actual retry
          timeoutRef.current = window.setTimeout(() => {
            setState(prev => ({ ...prev, retryAt: null }));
            executeAttempt(attempt + 1);
          }, delay);
        } else {
          // No more attempts - keep attemptCount for tracking
          setState(prev => ({ ...prev, isLoading: false, retryAt: null }));
        }
      }
    };

    await executeAttempt(0);
  }, [fn, clearTimer]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      clearTimer();
    };
  }, [clearTimer]);

  return {
    call,
    retryAt: state.retryAt,
    isLoading: state.isLoading,
    attemptCount: state.attemptCount,
  };
}
