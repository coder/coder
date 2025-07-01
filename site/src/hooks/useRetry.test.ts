import { act, renderHook } from "@testing-library/react";
import { useRetry } from "./useRetry";

// Mock timers
jest.useFakeTimers();

describe("useRetry", () => {
	const defaultOptions = {
		maxAttempts: 3,
		initialDelay: 1000,
		maxDelay: 8000,
		multiplier: 2,
	};

	let mockOnRetry: jest.Mock;

	beforeEach(() => {
		mockOnRetry = jest.fn();
		jest.clearAllTimers();
	});

	afterEach(() => {
		jest.clearAllMocks();
	});

	it("should initialize with correct default state", () => {
		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry }),
		);

		expect(result.current.isRetrying).toBe(false);
		expect(result.current.currentDelay).toBe(null);
		expect(result.current.attemptCount).toBe(0);
		expect(result.current.timeUntilNextRetry).toBe(null);
	});

	it("should start retrying when startRetrying is called", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry }),
		);

		act(() => {
			result.current.startRetrying();
		});

		expect(result.current.attemptCount).toBe(1);
		expect(result.current.isRetrying).toBe(true);

		// Wait for the retry to complete
		await act(async () => {
			await Promise.resolve();
		});

		expect(mockOnRetry).toHaveBeenCalledTimes(1);
		expect(result.current.isRetrying).toBe(false);
	});

	it("should calculate exponential backoff delays correctly", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry }),
		);

		act(() => {
			result.current.startRetrying();
		});

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		// Should schedule next retry with initial delay (1000ms)
		expect(result.current.currentDelay).toBe(1000);
		expect(result.current.timeUntilNextRetry).toBe(1000);

		// Fast forward to trigger second retry
		act(() => {
			jest.advanceTimersByTime(1000);
		});

		await act(async () => {
			await Promise.resolve();
		});

		// Should schedule third retry with doubled delay (2000ms)
		expect(result.current.currentDelay).toBe(2000);
	});

	it("should respect maximum delay", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const options = {
			...defaultOptions,
			maxDelay: 1500, // Lower max delay
			onRetry: mockOnRetry,
		};

		const { result } = renderHook(() => useRetry(options));

		act(() => {
			result.current.startRetrying();
		});

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		// Fast forward to trigger second retry
		act(() => {
			jest.advanceTimersByTime(1000);
		});

		await act(async () => {
			await Promise.resolve();
		});

		// Should cap at maxDelay instead of 2000ms
		expect(result.current.currentDelay).toBe(1500);
	});

	it("should stop retrying after max attempts", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry }),
		);

		act(() => {
			result.current.startRetrying();
		});

		// Simulate all retry attempts
		for (let i = 0; i < defaultOptions.maxAttempts; i++) {
			await act(async () => {
				await Promise.resolve();
			});

			if (i < defaultOptions.maxAttempts - 1) {
				// Fast forward to next retry
				act(() => {
					jest.advanceTimersByTime(result.current.currentDelay || 0);
				});
			}
		}

		expect(mockOnRetry).toHaveBeenCalledTimes(defaultOptions.maxAttempts);
		expect(result.current.attemptCount).toBe(defaultOptions.maxAttempts);
		expect(result.current.currentDelay).toBe(null);
		expect(result.current.timeUntilNextRetry).toBe(null);
	});

	it("should handle manual retry", async () => {
		mockOnRetry.mockRejectedValueOnce(new Error("Connection failed"));
		mockOnRetry.mockResolvedValueOnce(undefined);

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry }),
		);

		act(() => {
			result.current.startRetrying();
		});

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.currentDelay).toBe(1000);

		// Trigger manual retry before automatic retry
		act(() => {
			result.current.retry();
		});

		// Should cancel automatic retry
		expect(result.current.currentDelay).toBe(null);
		expect(result.current.timeUntilNextRetry).toBe(null);
		expect(result.current.isRetrying).toBe(true);

		await act(async () => {
			await Promise.resolve();
		});

		// Should succeed and reset state
		expect(result.current.attemptCount).toBe(0);
		expect(result.current.isRetrying).toBe(false);
	});

	it("should reset state when retry succeeds", async () => {
		mockOnRetry.mockRejectedValueOnce(new Error("Connection failed"));
		mockOnRetry.mockResolvedValueOnce(undefined);

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry }),
		);

		act(() => {
			result.current.startRetrying();
		});

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.attemptCount).toBe(1);

		// Fast forward to trigger second retry (which will succeed)
		act(() => {
			jest.advanceTimersByTime(1000);
		});

		await act(async () => {
			await Promise.resolve();
		});

		// Should reset all state
		expect(result.current.attemptCount).toBe(0);
		expect(result.current.isRetrying).toBe(false);
		expect(result.current.currentDelay).toBe(null);
		expect(result.current.timeUntilNextRetry).toBe(null);
	});

	it("should stop retrying when stopRetrying is called", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry }),
		);

		act(() => {
			result.current.startRetrying();
		});

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.currentDelay).toBe(1000);

		// Stop retrying
		act(() => {
			result.current.stopRetrying();
		});

		// Should reset all state
		expect(result.current.attemptCount).toBe(0);
		expect(result.current.isRetrying).toBe(false);
		expect(result.current.currentDelay).toBe(null);
		expect(result.current.timeUntilNextRetry).toBe(null);

		// Fast forward past when retry would have happened
		act(() => {
			jest.advanceTimersByTime(2000);
		});

		// Should not have triggered additional retries
		expect(mockOnRetry).toHaveBeenCalledTimes(1);
	});

	it("should update countdown timer correctly", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry }),
		);

		act(() => {
			result.current.startRetrying();
		});

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.timeUntilNextRetry).toBe(1000);

		// Advance time partially
		act(() => {
			jest.advanceTimersByTime(300);
		});

		// Should update countdown
		expect(result.current.timeUntilNextRetry).toBeLessThan(1000);
		expect(result.current.timeUntilNextRetry).toBeGreaterThan(0);
	});

	it("should handle the specified backoff configuration", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		// Test with the exact configuration from the issue
		const issueConfig = {
			onRetry: mockOnRetry,
			maxAttempts: 10,
			initialDelay: 1000, // 1 second
			maxDelay: 30000, // 30 seconds
			multiplier: 2,
		};

		const { result } = renderHook(() => useRetry(issueConfig));

		act(() => {
			result.current.startRetrying();
		});

		// Test first few delays
		const expectedDelays = [1000, 2000, 4000, 8000, 16000, 30000]; // Caps at 30000

		for (let i = 0; i < expectedDelays.length; i++) {
			await act(async () => {
				await Promise.resolve();
			});

			if (i < expectedDelays.length - 1) {
				expect(result.current.currentDelay).toBe(expectedDelays[i]);
				act(() => {
					jest.advanceTimersByTime(expectedDelays[i]);
				});
			}
		}
	});
});
