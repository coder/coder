import { renderHook } from "@testing-library/react";
import { useDebouncedFunction, useDebouncedValue } from "./debounce";

beforeAll(() => {
  jest.useFakeTimers();
  jest.spyOn(global, "setTimeout");
});

afterAll(() => {
  jest.useRealTimers();
  jest.clearAllMocks();
});

// Most UI tests should be structure from the user's experience, but just
// because these are more abstract, general-purpose hooks, it seemed harder to
// do that. Had to bring in some mocks
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

describe(`${useDebouncedValue.name}`, () => {
  it("Should immediately return out the exact same value (by reference) on mount", () => {
    const value = {};
    const { result } = renderDebouncedValue(value, 2000);

    expect(result.current).toBe(value);
    expect.hasAssertions();
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
    expect.hasAssertions();
  });

  it("Should resync after specified milliseconds pass with no change to arguments", async () => {
    const initialValue = false;
    const time = 5000;

    const { result, rerender } = renderDebouncedValue(initialValue, time);
    expect(result.current).toEqual(false);

    rerender({ value: !initialValue, time });
    await jest.runAllTimersAsync();

    expect(result.current).toEqual(true);
    expect.hasAssertions();
  });
});

describe(`${useDebouncedFunction.name}`, () => {
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
      expect.hasAssertions();
    });

    it("Resets any pending debounces if the timer argument changes", async () => {
      const time = 5000;
      let count = 0;
      const incrementCount = () => {
        count++;
      };

      const { result, rerender } = renderDebouncedFunction(
        incrementCount,
        time,
      );

      result.current.debounced();
      rerender({ callback: incrementCount, time: time + 1 });

      await jest.runAllTimersAsync();
      expect(count).toEqual(0);
      expect.hasAssertions();
    });
  });

  describe("debounced function", () => {
    it("Resolve the debounce after specified milliseconds pass with no other calls", async () => {
      let value = false;
      const { result } = renderDebouncedFunction(() => {
        value = !value;
      }, 100);

      result.current.debounced();

      await jest.runOnlyPendingTimersAsync();
      expect(value).toBe(true);
      expect.hasAssertions();
    });

    it("Always uses the most recent callback argument passed in (even if it switches while a debounce is queued)", async () => {
      let count = 0;
      const time = 500;

      const { result, rerender } = renderDebouncedFunction(() => {
        count = 1;
      }, time);

      result.current.debounced();
      rerender({
        callback: () => {
          count = 9999;
        },
        time,
      });

      await jest.runAllTimersAsync();
      expect(count).toEqual(9999);
      expect.hasAssertions();
    });

    it("Should reset the debounce timer with repeated calls to the method", async () => {
      let count = 0;
      const { result } = renderDebouncedFunction(() => {
        count++;
      }, 2000);

      for (let i = 0; i < 10; i++) {
        setTimeout(() => {
          result.current.debounced();
        }, i * 100);
      }

      await jest.runAllTimersAsync();
      expect(count).toBe(1);
      expect.hasAssertions();
    });
  });

  describe("cancelDebounce function", () => {
    it("Should be able to cancel a pending debounce", async () => {
      let count = 0;
      const { result } = renderDebouncedFunction(() => {
        count++;
      }, 2000);

      const { debounced, cancelDebounce } = result.current;
      debounced();
      cancelDebounce();

      await jest.runAllTimersAsync();
      expect(count).toEqual(0);
      expect.hasAssertions();
    });
  });
});
