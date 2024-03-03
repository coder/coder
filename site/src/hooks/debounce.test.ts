import { renderHook, waitFor } from "@testing-library/react";
import { useDebouncedFunction, useDebouncedValue } from "./debounce";

beforeAll(() => {
  jest.useFakeTimers();
  jest.spyOn(global, "setTimeout");
});

afterAll(() => {
  jest.useRealTimers();
  jest.clearAllMocks();
});

describe(`${useDebouncedValue.name}`, () => {
  function renderDebouncedValue<T = unknown>(value: T, time: number) {
    return renderHook(
      ({ value, time }: { value: T; time: number }) => {
        return useDebouncedValue(value, time);
      },
      {
        initialProps: { value, time },
      },
    );
  }

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

    await jest.advanceTimersByTimeAsync(time - 100);
    expect(result.current).toEqual(0);
  });

  it("Should resync after specified milliseconds pass with no change to arguments", async () => {
    const initialValue = false;
    const time = 5000;

    const { result, rerender } = renderDebouncedValue(initialValue, time);
    expect(result.current).toEqual(false);

    rerender({ value: !initialValue, time });
    await jest.runAllTimersAsync();
    await waitFor(() => expect(result.current).toEqual(true));
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

  describe("hook", () => {
    it("Should provide stable function references across re-renders", () => {
      const time = 5000;
      const { result, rerender } = renderDebouncedFunction(jest.fn(), time);

      const { debounced: oldDebounced, cancelDebounce: oldCancel } =
        result.current;

      rerender({ callback: jest.fn(), time });
      const { debounced: newDebounced, cancelDebounce: newCancel } =
        result.current;

      expect(oldDebounced).toBe(newDebounced);
      expect(oldCancel).toBe(newCancel);
    });

    it("Resets any pending debounces if the timer argument changes", async () => {
      const time = 5000;
      const mockCallback = jest.fn();
      const { result, rerender } = renderDebouncedFunction(mockCallback, time);

      result.current.debounced();
      rerender({ callback: mockCallback, time: time + 1 });

      await jest.runAllTimersAsync();
      expect(mockCallback).not.toBeCalled();
    });
  });

  describe("debounced function", () => {
    it("Resolve the debounce after specified milliseconds pass with no other calls", async () => {
      const mockCallback = jest.fn();
      const { result } = renderDebouncedFunction(mockCallback, 100);
      result.current.debounced();

      await jest.runOnlyPendingTimersAsync();
      expect(mockCallback).toBeCalledTimes(1);
    });

    it("Always uses the most recent callback argument passed in (even if it switches while a debounce is queued)", async () => {
      const mockCallback1 = jest.fn();
      const mockCallback2 = jest.fn();
      const time = 500;

      const { result, rerender } = renderDebouncedFunction(mockCallback1, time);
      result.current.debounced();
      rerender({ callback: mockCallback2, time });

      await jest.runAllTimersAsync();
      expect(mockCallback1).not.toBeCalled();
      expect(mockCallback2).toBeCalledTimes(1);
    });

    it("Should reset the debounce timer with repeated calls to the method", async () => {
      const mockCallback = jest.fn();
      const { result } = renderDebouncedFunction(mockCallback, 2000);

      for (let i = 0; i < 10; i++) {
        setTimeout(() => {
          result.current.debounced();
        }, i * 100);
      }

      await jest.runAllTimersAsync();
      expect(mockCallback).toBeCalledTimes(1);
    });
  });

  describe("cancelDebounce function", () => {
    it("Should be able to cancel a pending debounce", async () => {
      const mockCallback = jest.fn();
      const { result } = renderDebouncedFunction(mockCallback, 2000);

      result.current.debounced();
      result.current.cancelDebounce();

      await jest.runAllTimersAsync();
      expect(mockCallback).not.toBeCalled();
    });
  });
});
