import { renderHook, waitFor, act } from "@testing-library/react";
import { useDebouncedFunction, useDebouncedValue } from "./debounce";

beforeAll(() => {
	vi.useFakeTimers();
	vi.spyOn(global, "setTimeout");
});

afterAll(() => {
	vi.useRealTimers();
	vi.clearAllMocks();
});

describe(useDebouncedValue.name, () => {
	function renderDebouncedValue<T>(value: T, time: number) {
		return renderHook(
			({ value, time }: { value: T; time: number }) => {
				return useDebouncedValue(value, time);
			},
			{
				initialProps: { value, time },
			},
		);
	}

	it("Should throw for non-nonnegative integer timeouts", () => {
		const invalidInputs: readonly number[] = [
			Number.NaN,
			Number.NEGATIVE_INFINITY,
			Number.POSITIVE_INFINITY,
			Math.PI,
			-42,
		];

		const dummyValue = false;
		for (const input of invalidInputs) {
			expect(() => {
				renderDebouncedValue(dummyValue, input);
			}).toThrow(
				`Invalid value ${input} for debounceTimeoutMs. Value must be an integer greater than or equal to zero.`,
			);
		}
	});

	it("Should immediately return out the exact same value (by reference) on mount", () => {
		const value = {};
		const { result } = renderDebouncedValue(value, 2000);
		expect(result.current).toBe(value);
	});

	it("Should not immediately resync state as the hook re-renders with new value argument", async () => {
		let value = 0;
		const time = 5000;

		const { result, rerender } = renderDebouncedValue(value, time);
		expect(result.current).toEqual(0);

		for (let i = 1; i <= 5; i++) {
			setTimeout(() => {
				value++;
				rerender({ value, time });
			}, i * 100);
		}

		await vi.advanceTimersByTimeAsync(time - 100);
		expect(result.current).toEqual(0);
	});

	it("Should resync after specified milliseconds pass with no change to arguments", async () => {
		const initialValue = false;
		const time = 5000;

		const { result, rerender } = renderDebouncedValue(initialValue, time);
		expect(result.current).toEqual(false);

		rerender({ value: !initialValue, time });

		// Act wrapper ensures React updates are handled properly
		await act(async () => {
			await vi.runAllTimersAsync();
		});

		expect(result.current).toEqual(true);
	});

	// Very important that we not do any async logic for this test
	it("Should immediately resync without any render/event loop delays if timeout is zero", () => {
		const initialValue = false;
		const time = 5000;

		const { result, rerender } = renderDebouncedValue(initialValue, time);
		expect(result.current).toEqual(false);

		// Just to be on the safe side, re-render once with the old timeout to
		// verify that nothing has been flushed yet
		rerender({ value: !initialValue, time });
		expect(result.current).toEqual(false);

		// Then do the real re-render once we know the coast is clear
		rerender({ value: !initialValue, time: 0 });
		expect(result.current).toBe(true);
	});
});

describe(`${useDebouncedFunction.name}`, () => {
	function renderDebouncedFunction<Args extends unknown[]>(
		callbackArg: (...args: Args) => void | Promise<void>,
		time: number,
	) {
		return renderHook(
			({ callback, time }: { callback: typeof callbackArg; time: number }) => {
				return useDebouncedFunction<Args>(callback, time);
			},
			{
				initialProps: { callback: callbackArg, time },
			},
		);
	}

	describe("input validation", () => {
		it("Should throw for non-nonnegative integer timeouts", () => {
			const invalidInputs: readonly number[] = [
				Number.NaN,
				Number.NEGATIVE_INFINITY,
				Number.POSITIVE_INFINITY,
				Math.PI,
				-42,
			];

			const dummyFunction = vi.fn();
			for (const input of invalidInputs) {
				expect(() => {
					renderDebouncedFunction(dummyFunction, input);
				}).toThrow(
					`Invalid value ${input} for debounceTimeoutMs. Value must be an integer greater than or equal to zero.`,
				);
			}
		});
	});

	describe("hook", () => {
		it("Should provide stable function references across re-renders", () => {
			const time = 5000;
			const { result, rerender } = renderDebouncedFunction(vi.fn(), time);

			const { debounced: oldDebounced, cancelDebounce: oldCancel } =
				result.current;

			rerender({ callback: vi.fn(), time });
			const { debounced: newDebounced, cancelDebounce: newCancel } =
				result.current;

			expect(oldDebounced).toBe(newDebounced);
			expect(oldCancel).toBe(newCancel);
		});

		it("Resets any pending debounces if the timer argument changes", async () => {
			const time = 5000;
			const mockCallback = vi.fn();
			const { result, rerender } = renderDebouncedFunction(mockCallback, time);

			result.current.debounced();
			rerender({ callback: mockCallback, time: time + 1 });

			await vi.runAllTimersAsync();
			expect(mockCallback).not.toBeCalled();
		});
	});

	describe("debounced function", () => {
		it("Resolve the debounce after specified milliseconds pass with no other calls", async () => {
			const mockCallback = vi.fn();
			const { result } = renderDebouncedFunction(mockCallback, 100);
			result.current.debounced();

			await vi.runOnlyPendingTimersAsync();
			expect(mockCallback).toBeCalledTimes(1);
		});

		it("Always uses the most recent callback argument passed in (even if it switches while a debounce is queued)", async () => {
			const mockCallback1 = vi.fn();
			const mockCallback2 = vi.fn();
			const time = 500;

			const { result, rerender } = renderDebouncedFunction(mockCallback1, time);
			result.current.debounced();
			rerender({ callback: mockCallback2, time });

			await vi.runAllTimersAsync();
			expect(mockCallback1).not.toBeCalled();
			expect(mockCallback2).toBeCalledTimes(1);
		});

		it("Should reset the debounce timer with repeated calls to the method", async () => {
			const mockCallback = vi.fn();
			const { result } = renderDebouncedFunction(mockCallback, 2000);

			for (let i = 0; i < 10; i++) {
				setTimeout(() => {
					result.current.debounced();
				}, i * 100);
			}

			await vi.runAllTimersAsync();
			expect(mockCallback).toBeCalledTimes(1);
		});
	});

	describe("cancelDebounce function", () => {
		it("Should be able to cancel a pending debounce", async () => {
			const mockCallback = vi.fn();
			const { result } = renderDebouncedFunction(mockCallback, 2000);

			result.current.debounced();
			result.current.cancelDebounce();

			await vi.runAllTimersAsync();
			expect(mockCallback).not.toBeCalled();
		});
	});
});
