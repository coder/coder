import { type DependencyList } from "react";
import { type ClassName, useClassName } from "./useClassName";
import { type RenderHookOptions, renderHook } from "@testing-library/react";
import { ThemeProvider } from "@emotion/react";
import { ThemeOverride } from "contexts/ThemeProvider";
import themes from "theme";

/**
 * Treating the string that Emotion generates for the hook as an opaque value.
 * Trying to make assertions on the format could lead to flakier tests.
 */
describe(useClassName.name, () => {
  type Props = Readonly<{ styles: ClassName; deps: DependencyList }>;

  function renderUseClassNames(
    styles: ClassName,
    deps: DependencyList,
    options: Omit<RenderHookOptions<Props>, "initialProps"> = {},
  ) {
    return renderHook<string, Props>(
      /* eslint-disable-next-line react-hooks/exhaustive-deps --
         Disabling in text context to make setup easier; disabling the rule
         should still be treated as a bad idea in general */
      ({ styles, deps }) => useClassName(styles, deps),
      { initialProps: { styles, deps }, ...options },
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
    expect(mockCallback).toBeCalledTimes(1);

    const initialResult = result.current;
    rerender({ styles: mockCallback, deps: [color] });
    expect(result.current).toEqual(initialResult);
    expect(mockCallback).toBeCalledTimes(1);

    color = "blue";
    rerender({ styles: mockCallback, deps: [color] });
    expect(result.current).not.toEqual(initialResult);
    expect(mockCallback).toBeCalledTimes(2);
  });

  it("Should use the closest theme provider available in the React tree", () => {
    const className: ClassName = (css, theme) => css`
      background-color: ${theme.roles.active.background};
    `;

    const { result: result1 } = renderUseClassNames(className, [], {
      wrapper: ({ children }) => (
        <ThemeProvider theme={themes.dark}>{children}</ThemeProvider>
      ),
    });

    const { result: result2 } = renderUseClassNames(className, [], {
      wrapper: ({ children }) => (
        <ThemeProvider theme={themes.dark}>
          <ThemeOverride theme={themes.light}>{children}</ThemeOverride>
        </ThemeProvider>
      ),
    });

    const { result: result3 } = renderUseClassNames(className, [], {
      wrapper: ({ children }) => (
        <ThemeProvider theme={themes.dark}>
          <ThemeOverride theme={themes.light}>
            <ThemeOverride theme={themes.darkBlue}>
              <ThemeOverride theme={themes.light}>
                <ThemeOverride theme={themes.dark}>{children}</ThemeOverride>
              </ThemeOverride>
            </ThemeOverride>
          </ThemeOverride>
        </ThemeProvider>
      ),
    });

    expect(result1.current).not.toEqual(result2.current);
    expect(result1.current).toEqual(result3.current);
  });
});
