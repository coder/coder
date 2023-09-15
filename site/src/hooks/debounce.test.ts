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

// The general approach is to structure the tests from the user's experience,
// but just because these are more abstract, general-purpose hooks, it seemed
// harder to do that. Had to bring in some mocks
function renderDebounceValue<T = unknown>(value: T, time: number) {
  return renderHook(
    ({ value, time }: { value: T; time: number }) => {
      return useDebouncedValue(value, time);
    },
    {
      initialProps: { value, time },
    },
  );
}

// Something in the React Testing Library types seems to be losing some of the
// type parameters for the returned-out debounced method. It has much better
// ergonomics/type inference when actually using it in a real React app
function renderDebounceFunction<
  Args extends unknown[],
  Fn extends (...args: Args) => void | Promise<void>,
>(callback: Fn, time: number) {
  return renderHook(
    ({ callback, time }: { callback: Fn; time: number }) => {
      return useDebouncedFunction<Args>(callback, time);
    },
    {
      initialProps: { callback, time },
    },
  );
}

describe(`${useDebouncedValue.name}`, () => {
  it("Should immediately return out the exact same value (by reference) on mount", () => {
    const value = {};
    const { result } = renderDebounceValue(value, 2000);

    expect(result.current).toBe(value);
    expect.hasAssertions();
  });

  it("Should not immediately resync as the source value changes", async () => {
    let value = 0;
    const time = 5000;

    const { result, rerender } = renderDebounceValue(value, time);
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

    const { result, rerender } = renderDebounceValue(initialValue, time);
    expect(result.current).toEqual(false);

    rerender({ value: !initialValue, time });
    await jest.runAllTimersAsync();

    expect(result.current).toEqual(true);
    expect.hasAssertions();
  });
});

describe(`${useDebouncedFunction.name}`, () => {
  describe("hook", () => {
    it("Should provide stable function references across all renders", () => {
      const time = 5000;
      const { result, rerender } = renderDebounceFunction(jest.fn(), time);

      const { debounced: oldDebounced, cancelDebounce: oldCancel } =
        result.current;

      rerender({ callback: jest.fn(), time });
      const { debounced: newDebounced, cancelDebounce: newCancel } =
        result.current;

      expect(oldDebounced).toBe(newDebounced);
      expect(oldCancel).toBe(newCancel);
      expect.hasAssertions();
    });

    it.skip("Resets any pending debounces if the timer argument changes", () => {
      expect.hasAssertions();
    });
  });

  describe("debounced function", () => {
    it.skip("Should be able to 'see' the most recent arguments across re-renders", () => {
      expect.hasAssertions();
    });

    it.skip("Should reset the debounce timer with repeated calls to the method", () => {
      expect.hasAssertions();
    });

    it.skip("Resolve the debounce after specified milliseconds pass with no other calls", () => {
      expect.hasAssertions();
    });
  });

  describe("cancelDebounce function", () => {
    it("Should be able to cancel a pending debounce at any time", async () => {
      let count = 0;
      const { result } = renderDebounceFunction(() => {
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
