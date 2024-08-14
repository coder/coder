import { renderHook } from "@testing-library/react";
import { useEffectEvent } from "./hookPolyfills";

describe(useEffectEvent.name, () => {
  function renderEffectEvent<TArgs extends unknown[], TReturn = unknown>(
    callbackArg: (...args: TArgs) => TReturn,
  ) {
    type Callback = typeof callbackArg;
    type Props = Readonly<{ callback: Callback }>;

    return renderHook<Callback, Props>(
      ({ callback }) => useEffectEvent(callback),
      { initialProps: { callback: callbackArg } },
    );
  }

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
    const mockCallback1 = jest.fn();
    const mockCallback2 = jest.fn();

    const { result, rerender } = renderEffectEvent(mockCallback1);
    rerender({ callback: mockCallback2 });

    result.current();
    expect(mockCallback1).not.toBeCalled();
    expect(mockCallback2).toBeCalledTimes(1);
  });
});
