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
		enabled: false, // Default to disabled for most tests
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

	it("should start retrying when enabled is true", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry, enabled: true }),
		);

		expect(result.current.attemptCount).toBe(1);
		expect(result.current.isRetrying).toBe(true);

		// Wait for the retry to complete
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.isRetrying).toBe(false);
		expect(mockOnRetry).toHaveBeenCalledTimes(1);
	});

	it("should calculate exponential backoff delays correctly", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry, enabled: true }),
		);

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

		// Wait for second retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		// Should schedule next retry with doubled delay (2000ms)
		expect(result.current.currentDelay).toBe(2000);
		expect(result.current.timeUntilNextRetry).toBe(2000);
	});

	it("should respect maximum delay", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({
				...defaultOptions,
				onRetry: mockOnRetry,
				enabled: true,
				maxAttempts: 10,
				initialDelay: 4000,
				maxDelay: 8000,
			}),
		);

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		// Fast forward through multiple retries to reach max delay
		act(() => {
			jest.advanceTimersByTime(4000); // First retry (4000ms)
		});

		await act(async () => {
			await Promise.resolve();
		});

		act(() => {
			jest.advanceTimersByTime(8000); // Second retry (8000ms - capped at maxDelay)
		});

		await act(async () => {
			await Promise.resolve();
		});

		// Should not exceed max delay
		expect(result.current.currentDelay).toBe(8000);
	});

	it("should stop retrying after max attempts", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry, enabled: true }),
		);

		// Wait for first retry
		await act(async () => {
			await Promise.resolve();
		});

		// Fast forward through all retries
		for (let i = 0; i < defaultOptions.maxAttempts - 1; i++) {
			act(() => {
				jest.advanceTimersByTime(10000); // Advance past any delay
			});

			await act(async () => {
				await Promise.resolve();
			});
		}

		// Should have reached max attempts
		expect(result.current.attemptCount).toBe(defaultOptions.maxAttempts);
		expect(result.current.currentDelay).toBe(null);
		expect(result.current.timeUntilNextRetry).toBe(null);
	});

	it("should handle manual retry", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry, enabled: true }),
		);

		// Wait for first retry to fail and schedule next
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.timeUntilNextRetry).toBe(1000);

		// Trigger manual retry
		act(() => {
			result.current.retry();
		});

		// Should cancel scheduled retry and start immediately
		expect(result.current.timeUntilNextRetry).toBe(null);
		expect(result.current.isRetrying).toBe(true);

		await act(async () => {
			await Promise.resolve();
		});

		expect(mockOnRetry).toHaveBeenCalledTimes(2); // Initial + manual
	});

	it("should reset state when retry succeeds", async () => {
		mockOnRetry
			.mockRejectedValueOnce(new Error("Connection failed"))
			.mockResolvedValueOnce(undefined);

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry, enabled: true }),
		);

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.attemptCount).toBe(1);
		expect(result.current.timeUntilNextRetry).toBe(1000);

		// Fast forward to trigger second retry (which will succeed)
		act(() => {
			jest.advanceTimersByTime(1000);
		});

		await act(async () => {
			await Promise.resolve();
		});

		// Should reset to initial state after success
		expect(result.current.attemptCount).toBe(0);
		expect(result.current.isRetrying).toBe(false);
		expect(result.current.currentDelay).toBe(null);
		expect(result.current.timeUntilNextRetry).toBe(null);
	});

	it("should stop retrying when enabled becomes false", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result, rerender } = renderHook(
			({ enabled }) =>
				useRetry({ ...defaultOptions, onRetry: mockOnRetry, enabled }),
			{ initialProps: { enabled: true } },
		);

		// Wait for first retry to fail and schedule next
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.attemptCount).toBe(1);
		expect(result.current.timeUntilNextRetry).toBe(1000);

		// Disable retrying
		act(() => {
			rerender({ enabled: false });
		});

		// Should reset state
		expect(result.current.attemptCount).toBe(0);
		expect(result.current.isRetrying).toBe(false);
		expect(result.current.currentDelay).toBe(null);
		expect(result.current.timeUntilNextRetry).toBe(null);
	});

	it("should update countdown timer correctly", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const { result } = renderHook(() =>
			useRetry({ ...defaultOptions, onRetry: mockOnRetry, enabled: true }),
		);

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.timeUntilNextRetry).toBe(1000);

		// Advance timer partially
		act(() => {
			jest.advanceTimersByTime(500);
		});

		// Should update countdown
		expect(result.current.timeUntilNextRetry).toBe(500);
	});

	it("should handle the specified backoff configuration", async () => {
		mockOnRetry.mockRejectedValue(new Error("Connection failed"));

		const customOptions = {
			onRetry: mockOnRetry,
			maxAttempts: 10,
			initialDelay: 1000,
			maxDelay: 30000,
			multiplier: 2,
			enabled: true,
		};

		const { result } = renderHook(() => useRetry(customOptions));

		// Wait for first retry to fail
		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.attemptCount).toBe(1);
		expect(result.current.currentDelay).toBe(1000);

		// Fast forward to trigger second retry
		act(() => {
			jest.advanceTimersByTime(1000);
		});

		await act(async () => {
			await Promise.resolve();
		});

		expect(result.current.attemptCount).toBe(2);
		expect(result.current.currentDelay).toBe(2000);
	});
});
