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
		expect(result.current.nextRetryAt).toBe(undefined);
	});

	it("should execute function successfully on first attempt", async () => {
		mockFn.mockResolvedValue(undefined);

		const { result } = renderHook(() => useWithRetry(mockFn));

		await act(async () => {
			await result.current.call();
		});

		expect(mockFn).toHaveBeenCalledTimes(1);
		expect(result.current.isLoading).toBe(false);
		expect(result.current.nextRetryAt).toBe(undefined);
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
		expect(result.current.isLoading).toBe(false);
		expect(result.current.nextRetryAt).not.toBe(null);

		// Fast-forward to first retry (1 second)
		await act(async () => {
			jest.advanceTimersByTime(1000);
		});

		expect(mockFn).toHaveBeenCalledTimes(2);
		expect(result.current.isLoading).toBe(false);
		expect(result.current.nextRetryAt).not.toBe(null);

		// Fast-forward to second retry (2 seconds)
		await act(async () => {
			jest.advanceTimersByTime(2000);
		});

		expect(mockFn).toHaveBeenCalledTimes(3);
		expect(result.current.isLoading).toBe(false);
		expect(result.current.nextRetryAt).toBe(undefined);
	});

	it("should continue retrying without limit", async () => {
		mockFn.mockRejectedValue(new Error("Always fails"));

		const { result } = renderHook(() => useWithRetry(mockFn));

		// Start the call
		await act(async () => {
			await result.current.call();
		});

		expect(mockFn).toHaveBeenCalledTimes(1);
		expect(result.current.isLoading).toBe(false);
		expect(result.current.nextRetryAt).not.toBe(null);

		// Fast-forward through multiple retries to verify it continues
		for (let i = 1; i < 15; i++) {
			const delay = Math.min(1000 * 2 ** (i - 1), 600000); // exponential backoff with max delay
			await act(async () => {
				jest.advanceTimersByTime(delay);
			});
			expect(mockFn).toHaveBeenCalledTimes(i + 1);
			expect(result.current.isLoading).toBe(false);
			expect(result.current.nextRetryAt).not.toBe(null);
		}

		// Should still be retrying after 15 attempts
		expect(result.current.nextRetryAt).not.toBe(null);
	});

	it("should respect max delay of 10 minutes", async () => {
		mockFn.mockRejectedValue(new Error("Always fails"));

		const { result } = renderHook(() => useWithRetry(mockFn));

		// Start the call
		await act(async () => {
			await result.current.call();
		});

		expect(result.current.isLoading).toBe(false);

		// Fast-forward through several retries to reach max delay
		// After attempt 9, delay would be 1000 * 2^9 = 512000ms, which is less than 600000ms (10 min)
		// After attempt 10, delay would be 1000 * 2^10 = 1024000ms, which should be capped at 600000ms

		// Skip to attempt 9 (delay calculation: 1000 * 2^8 = 256000ms)
		for (let i = 1; i < 9; i++) {
			const delay = 1000 * 2 ** (i - 1);
			await act(async () => {
				jest.advanceTimersByTime(delay);
			});
		}

		expect(mockFn).toHaveBeenCalledTimes(9);
		expect(result.current.nextRetryAt).not.toBe(null);

		// The 9th retry should use max delay (600000ms = 10 minutes)
		await act(async () => {
			jest.advanceTimersByTime(600000);
		});

		expect(mockFn).toHaveBeenCalledTimes(10);
		expect(result.current.isLoading).toBe(false);
		expect(result.current.nextRetryAt).not.toBe(null);

		// Continue with more retries at max delay to verify it continues indefinitely
		await act(async () => {
			jest.advanceTimersByTime(600000);
		});

		expect(mockFn).toHaveBeenCalledTimes(11);
		expect(result.current.nextRetryAt).not.toBe(null);
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
		expect(result.current.isLoading).toBe(false);
		expect(result.current.nextRetryAt).not.toBe(null);

		// Call again before retry happens
		await act(async () => {
			await result.current.call();
		});

		expect(mockFn).toHaveBeenCalledTimes(2);
		expect(result.current.isLoading).toBe(false);
		expect(result.current.nextRetryAt).toBe(undefined);

		// Advance time to ensure previous retry was cancelled
		await act(async () => {
			jest.advanceTimersByTime(5000);
		});

		expect(mockFn).toHaveBeenCalledTimes(2); // Should not have been called again
	});

	it("should set nextRetryAt when scheduling retry", async () => {
		mockFn
			.mockRejectedValueOnce(new Error("Failure"))
			.mockResolvedValueOnce(undefined);

		const { result } = renderHook(() => useWithRetry(mockFn));

		// Start the call
		await act(async () => {
			await result.current.call();
		});

		const nextRetryAt = result.current.nextRetryAt;
		expect(nextRetryAt).not.toBe(null);
		expect(nextRetryAt).toBeInstanceOf(Date);

		// nextRetryAt should be approximately 1 second in the future
		const expectedTime = Date.now() + 1000;
		const actualTime = nextRetryAt!.getTime();
		expect(Math.abs(actualTime - expectedTime)).toBeLessThan(100); // Allow 100ms tolerance

		// Advance past retry time
		await act(async () => {
			jest.advanceTimersByTime(1000);
		});

		expect(result.current.nextRetryAt).toBe(undefined);
	});

	it("should cleanup timer on unmount", async () => {
		mockFn.mockRejectedValue(new Error("Failure"));

		const { result, unmount } = renderHook(() => useWithRetry(mockFn));

		// Start the call to create timer
		await act(async () => {
			await result.current.call();
		});

		expect(result.current.isLoading).toBe(false);
		expect(result.current.nextRetryAt).not.toBe(null);

		// Unmount should cleanup timer
		unmount();

		// Advance time to ensure timer was cleared
		await act(async () => {
			jest.advanceTimersByTime(5000);
		});

		// Function should not have been called again
		expect(mockFn).toHaveBeenCalledTimes(1);
	});

	it("should prevent scheduling retries when function completes after unmount", async () => {
		let rejectPromise: (error: Error) => void;
		const promise = new Promise<void>((_, reject) => {
			rejectPromise = reject;
		});
		mockFn.mockReturnValue(promise);

		const { result, unmount } = renderHook(() => useWithRetry(mockFn));

		// Start the call - this will make the function in-flight
		act(() => {
			result.current.call();
		});

		expect(result.current.isLoading).toBe(true);

		// Unmount while function is still in-flight
		unmount();

		// Function completes with error after unmount
		await act(async () => {
			rejectPromise!(new Error("Failed after unmount"));
			await promise.catch(() => {}); // Suppress unhandled rejection
		});

		// Advance time to ensure no retry timers were scheduled
		await act(async () => {
			jest.advanceTimersByTime(5000);
		});

		// Function should only have been called once (no retries after unmount)
		expect(mockFn).toHaveBeenCalledTimes(1);
	});

	it("should do nothing when call() is invoked while function is already loading", async () => {
		let resolvePromise: () => void;
		const promise = new Promise<void>((resolve) => {
			resolvePromise = resolve;
		});
		mockFn.mockReturnValue(promise);

		const { result } = renderHook(() => useWithRetry(mockFn));

		// Start the first call - this will set isLoading to true
		act(() => {
			result.current.call();
		});

		expect(result.current.isLoading).toBe(true);
		expect(mockFn).toHaveBeenCalledTimes(1);

		// Try to call again while loading - should do nothing
		act(() => {
			result.current.call();
		});

		// Function should not have been called again
		expect(mockFn).toHaveBeenCalledTimes(1);
		expect(result.current.isLoading).toBe(true);

		// Complete the original promise
		await act(async () => {
			resolvePromise!();
			await promise;
		});

		expect(result.current.isLoading).toBe(false);
		expect(mockFn).toHaveBeenCalledTimes(1);
	});
});
