import { renderHook } from "@testing-library/react";
import { useEffectEvent } from "./hookPolyfills";

function renderEffectEvent<TArgs extends unknown[], TReturn = unknown>(
  callbackArg: (...args: TArgs) => TReturn,
) {
  return renderHook(
    ({ callback }: { callback: typeof callbackArg }) => {
      return useEffectEvent(callback);
    },
    {
      initialProps: { callback: callbackArg },
    },
  );
}

describe(`${useEffectEvent.name}`, () => {
  it("Should maintain a stable reference across all renders", () => {
    const callback = jest.fn();
    const { result, rerender } = renderEffectEvent(callback);

    const firstResult = result.current;
    for (let i = 0; i < 5; i++) {
      rerender({ callback });
    }

    expect(result.current).toBe(firstResult);
    expect.hasAssertions();
  });

  it("Should always call the most recent callback passed in", () => {
    let value: "A" | "B" | "C" = "A";
    const flipToB = () => {
      value = "B";
    };

    const flipToC = () => {
      value = "C";
    };

    const { result, rerender } = renderEffectEvent(flipToB);
    rerender({ callback: flipToC });

    result.current();
    expect(value).toEqual("C");
    expect.hasAssertions();
  });
});
