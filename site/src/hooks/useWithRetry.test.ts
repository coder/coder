import { act, renderHook } from "@testing-library/react";
import { useWithRetry } from "./useWithRetry";

// Mock timers
jest.useFakeTimers();

describe("useWithRetry", () => {
  let mockFn: jest.Mock;

  beforeEach(() => {
    mockFn = jest.fn();
    jest.clearAllTimers();
  });

  afterEach(() => {
    jest.clearAllMocks();
  });

  it("should initialize with correct default state", () => {
    const { result } = renderHook(() => useWithRetry(mockFn));

    expect(result.current.isLoading).toBe(false);
    expect(result.current.retryAt).toBe(null);
  });

  it("should execute function successfully on first attempt", async () => {
    mockFn.mockResolvedValue(undefined);

    const { result } = renderHook(() => useWithRetry(mockFn));

    await act(async () => {
      await result.current.call();
    });

    expect(mockFn).toHaveBeenCalledTimes(1);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.retryAt).toBe(null);
  });

  it("should set isLoading to true during execution", async () => {
    let resolvePromise: () => void;
    const promise = new Promise<void>((resolve) => {
      resolvePromise = resolve;
    });
    mockFn.mockReturnValue(promise);

    const { result } = renderHook(() => useWithRetry(mockFn));

    act(() => {
      result.current.call();
    });

    expect(result.current.isLoading).toBe(true);

    await act(async () => {
      resolvePromise!();
      await promise;
    });

    expect(result.current.isLoading).toBe(false);
  });

  it("should retry on failure with exponential backoff", async () => {
    mockFn
      .mockRejectedValueOnce(new Error("First failure"))
      .mockRejectedValueOnce(new Error("Second failure"))
      .mockResolvedValueOnce(undefined);

    const { result } = renderHook(() => useWithRetry(mockFn));

    // Start the call
    await act(async () => {
      await result.current.call();
    });

    expect(mockFn).toHaveBeenCalledTimes(1);
    expect(result.current.isLoading).toBe(true);
    expect(result.current.retryAt).not.toBe(null);

    // Fast-forward to first retry (1 second)
    await act(async () => {
      jest.advanceTimersByTime(1000);
    });

    expect(mockFn).toHaveBeenCalledTimes(2);
    expect(result.current.isLoading).toBe(true);
    expect(result.current.retryAt).not.toBe(null);

    // Fast-forward to second retry (2 seconds)
    await act(async () => {
      jest.advanceTimersByTime(2000);
    });

    expect(mockFn).toHaveBeenCalledTimes(3);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.retryAt).toBe(null);
  });

  it("should stop retrying after max attempts", async () => {
    mockFn.mockRejectedValue(new Error("Always fails"));

    const { result } = renderHook(() =>
      useWithRetry(mockFn, { maxAttempts: 2 }),
    );

    // Start the call
    await act(async () => {
      await result.current.call();
    });

    expect(mockFn).toHaveBeenCalledTimes(1);
    expect(result.current.isLoading).toBe(true);

    // Fast-forward to first retry
    await act(async () => {
      jest.advanceTimersByTime(1000);
    });

    expect(mockFn).toHaveBeenCalledTimes(2);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.retryAt).toBe(null);
  });

  it("should use custom retry options", async () => {
    mockFn
      .mockRejectedValueOnce(new Error("First failure"))
      .mockResolvedValueOnce(undefined);

    const { result } = renderHook(() =>
      useWithRetry(mockFn, {
        initialDelay: 500,
        multiplier: 3,
        maxAttempts: 2,
      }),
    );

    // Start the call
    await act(async () => {
      await result.current.call();
    });

    expect(mockFn).toHaveBeenCalledTimes(1);
    expect(result.current.isLoading).toBe(true);
    expect(result.current.retryAt).not.toBe(null);

    // Fast-forward by custom initial delay (500ms)
    await act(async () => {
      jest.advanceTimersByTime(500);
    });

    expect(mockFn).toHaveBeenCalledTimes(2);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.retryAt).toBe(null);
  });

  it("should respect max delay", async () => {
    mockFn.mockRejectedValue(new Error("Always fails"));

    const { result } = renderHook(() =>
      useWithRetry(mockFn, {
        initialDelay: 1000,
        multiplier: 10,
        maxDelay: 2000,
        maxAttempts: 3,
      }),
    );

    // Start the call
    await act(async () => {
      await result.current.call();
    });

    expect(result.current.isLoading).toBe(true);

    // First retry should be at 1000ms (initial delay)
    await act(async () => {
      jest.advanceTimersByTime(1000);
    });

    expect(mockFn).toHaveBeenCalledTimes(2);

    // Second retry should be at 2000ms (max delay, not 10000ms)
    await act(async () => {
      jest.advanceTimersByTime(2000);
    });

    expect(mockFn).toHaveBeenCalledTimes(3);
    expect(result.current.isLoading).toBe(false);
  });

  it("should cancel previous retry when call is invoked again", async () => {
    mockFn
      .mockRejectedValueOnce(new Error("First failure"))
      .mockResolvedValueOnce(undefined);

    const { result } = renderHook(() => useWithRetry(mockFn));

    // Start the first call
    await act(async () => {
      await result.current.call();
    });

    expect(mockFn).toHaveBeenCalledTimes(1);
    expect(result.current.isLoading).toBe(true);
    expect(result.current.retryAt).not.toBe(null);

    // Call again before retry happens
    await act(async () => {
      await result.current.call();
    });

    expect(mockFn).toHaveBeenCalledTimes(2);
    expect(result.current.isLoading).toBe(false);
    expect(result.current.retryAt).toBe(null);

    // Advance time to ensure previous retry was cancelled
    await act(async () => {
      jest.advanceTimersByTime(5000);
    });

    expect(mockFn).toHaveBeenCalledTimes(2); // Should not have been called again
  });

  it("should update retryAt countdown", async () => {
    mockFn.mockRejectedValue(new Error("Failure"));

    const { result } = renderHook(() =>
      useWithRetry(mockFn, { initialDelay: 1000 }),
    );

    // Start the call
    await act(async () => {
      await result.current.call();
    });

    const initialRetryAt = result.current.retryAt;
    expect(initialRetryAt).not.toBe(null);

    // Advance time by 100ms (countdown update interval)
    await act(async () => {
      jest.advanceTimersByTime(100);
    });

    // retryAt should still be set but countdown should be updating
    expect(result.current.retryAt).not.toBe(null);

    // Advance to just before retry time
    await act(async () => {
      jest.advanceTimersByTime(850);
    });

    expect(result.current.retryAt).not.toBe(null);

    // Advance past retry time
    await act(async () => {
      jest.advanceTimersByTime(100);
    });

    expect(result.current.retryAt).toBe(null);
  });

  it("should cleanup timers on unmount", async () => {
    mockFn.mockRejectedValue(new Error("Failure"));

    const { result, unmount } = renderHook(() => useWithRetry(mockFn));

    // Start the call to create timers
    await act(async () => {
      await result.current.call();
    });

    expect(result.current.isLoading).toBe(true);
    expect(result.current.retryAt).not.toBe(null);

    // Unmount should cleanup timers
    unmount();

    // Advance time to ensure timers were cleared
    await act(async () => {
      jest.advanceTimersByTime(5000);
    });

    // Function should not have been called again
    expect(mockFn).toHaveBeenCalledTimes(1);
  });
});
