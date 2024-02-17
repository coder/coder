import { type DependencyList } from "react";
import { type ClassName, useClassName } from "./useClassName";
import { renderHook } from "@testing-library/react";

/**
 * Treating the string that Emotion generates for the hook as an opaque value.
 * Trying to make assertions on the format could lead to flakier tests.
 */
describe(useClassName.name, () => {
  function renderUseClassNames(styles: ClassName, deps: DependencyList) {
    type Props = Readonly<{ styles: ClassName; deps: DependencyList }>;
    return renderHook<string, Props>(
      /* eslint-disable-next-line react-hooks/exhaustive-deps --
         Disabling in text context to make setup easier; disabling the rule
         should be treated as a bad idea in general
      */
      ({ styles, deps }) => useClassName(styles, deps),
      { initialProps: { styles, deps } },
    );
  }

  test("Two separate hook calls with the same styles should produce the same string (regardless of function references)", () => {
    const func1: ClassName = (css) => css`
      background-color: chartreuse;
    `;
    const func2: ClassName = (css) => css`
      background-color: chartreuse;
    `;

    const { result: result1 } = renderUseClassNames(func1, []);
    const { result: result2 } = renderUseClassNames(func2, []);
    expect(result1.current).toBe(result2.current);
  });

  it("Only re-evaluates the styles callback if the dependencies change during re-renders", () => {
    let color: "red" | "blue" = "red";
    const className: ClassName = (css) => css`
      color: ${color};
    `;

    const mockCallback = jest.fn(className);
    const { result, rerender } = renderUseClassNames(mockCallback, [color]);
    const firstResult = result.current;

    rerender({ styles: mockCallback, deps: [color] });
    expect(result.current).toEqual(firstResult);
    expect(mockCallback).toBeCalledTimes(1);

    color = "blue";
    rerender({ styles: mockCallback, deps: [color] });
    expect(result.current).not.toEqual(firstResult);
    expect(mockCallback).toBeCalledTimes(2);
  });

  it.skip("Should use the closest theme provider available in the React tree", () => {
    expect.hasAssertions();
  });
});
