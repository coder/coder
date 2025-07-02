import { useCallback, useEffect, useRef, useState } from "react";

// Configuration constants
const DEFAULT_MAX_ATTEMPTS = 3;
const DEFAULT_INITIAL_DELAY = 1000; // 1 second
const DEFAULT_MAX_DELAY = 8000; // 8 seconds
const DEFAULT_MULTIPLIER = 2;
const COUNTDOWN_UPDATE_INTERVAL = 100; // Update countdown every 100ms

interface UseWithRetryResult {
  // Executes the function received
  call: () => Promise<void>;
  retryAt: Date | null;
  isLoading: boolean;
}

interface UseWithRetryOptions {
  maxAttempts?: number;
  initialDelay?: number;
  maxDelay?: number;
  multiplier?: number;
}

/**
 * Hook that wraps a function with automatic retry functionality
 * Provides a simple interface for executing functions with exponential backoff retry
 */
export function useWithRetry(
  fn: () => Promise<void>,
  options: UseWithRetryOptions = {},
): UseWithRetryResult {
  const {
    maxAttempts = DEFAULT_MAX_ATTEMPTS,
    initialDelay = DEFAULT_INITIAL_DELAY,
    maxDelay = DEFAULT_MAX_DELAY,
    multiplier = DEFAULT_MULTIPLIER,
  } = options;

  const [isLoading, setIsLoading] = useState(false);
  const [retryAt, setRetryAt] = useState<Date | null>(null);
  const [attemptCount, setAttemptCount] = useState(0);

  const timeoutRef = useRef<number | null>(null);
  const countdownRef = useRef<number | null>(null);

  const clearTimers = useCallback(() => {
    if (timeoutRef.current) {
      window.clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
    if (countdownRef.current) {
      window.clearInterval(countdownRef.current);
      countdownRef.current = null;
    }
  }, []);

  const calculateDelay = useCallback(
    (attempt: number): number => {
      const delay = initialDelay * multiplier ** attempt;
      return Math.min(delay, maxDelay);
    },
    [initialDelay, multiplier, maxDelay],
  );

  const scheduleRetry = useCallback(
    (attempt: number) => {
      if (attempt >= maxAttempts) {
        setIsLoading(false);
        setRetryAt(null);
        return;
      }

      const delay = calculateDelay(attempt);
      const retryTime = new Date(Date.now() + delay);
      setRetryAt(retryTime);

      // Update countdown every 100ms for smooth UI updates
      countdownRef.current = window.setInterval(() => {
        const now = Date.now();
        const timeLeft = retryTime.getTime() - now;
        
        if (timeLeft <= 0) {
          clearTimers();
          setRetryAt(null);
        }
      }, COUNTDOWN_UPDATE_INTERVAL);

      // Schedule the actual retry
      timeoutRef.current = window.setTimeout(() => {
        setRetryAt(null);
        executeFunction(attempt + 1);
      }, delay);
    },
    [maxAttempts, calculateDelay, clearTimers],
  );

  const executeFunction = useCallback(
    async (attempt: number = 0) => {
      setIsLoading(true);
      setAttemptCount(attempt);

      try {
        await fn();
        // Success - reset everything
        setIsLoading(false);
        setRetryAt(null);
        setAttemptCount(0);
        clearTimers();
      } catch (error) {
        // Failure - schedule retry if attempts remaining
        if (attempt < maxAttempts) {
          scheduleRetry(attempt);
        } else {
          // No more attempts - reset state
          setIsLoading(false);
          setRetryAt(null);
          setAttemptCount(0);
        }
      }
    },
    [fn, maxAttempts, scheduleRetry, clearTimers],
  );

  const call = useCallback(() => {
    // Cancel any existing retry and start fresh
    clearTimers();
    setRetryAt(null);
    return executeFunction(0);
  }, [executeFunction, clearTimers]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      clearTimers();
    };
  }, [clearTimers]);

  return {
    call,
    retryAt,
    isLoading,
  };
}
