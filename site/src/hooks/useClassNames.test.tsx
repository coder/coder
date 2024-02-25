import { type StylesFunction, makeClassNames } from "./useClassNames";
import { RenderHookOptions, renderHook } from "@testing-library/react";

import { css as emotionCss } from "@emotion/css";
import { type Theme, ThemeProvider } from "@emotion/react";
import { ThemeOverride } from "contexts/ThemeProvider";
import themes from "theme";

type HookProps = Readonly<{
  color: "red" | "blue";
}>;

function setupUseClassNames<TKey extends string = string>(
  implementation: StylesFunction<TKey, HookProps>,
  options?: Omit<RenderHookOptions<HookProps>, "initialProps">,
) {
  const mockCallback = jest.fn(implementation);
  const useClassNames = makeClassNames(mockCallback);
  const { wrapper, ...delegatedOptions } = options ?? {};
  type Result = ReturnType<typeof useClassNames>;

  const { result, rerender, unmount } = renderHook<Result, HookProps>(
    ({ color }) => useClassNames({ color }),
    {
      ...delegatedOptions,
      initialProps: { color: "red" },
      wrapper:
        wrapper ??
        (({ children }) => (
          <ThemeProvider theme={themes.dark}>{children}</ThemeProvider>
        )),
    },
  );

  return { result, rerender, unmount, mockCallback };
}

/**
 * Treating the string that Emotion generates for the hook as an opaque value.
 * Trying to make assertions on the format could lead to flakier tests.
 */
describe(makeClassNames.name, () => {
  test("Hook output should be fully deterministic based on theme and inputs", () => {
    const { result } = setupUseClassNames((css, theme) => ({
      class1: css`
        background-color: chartreuse;
      `,

      class2: css`
        background-color: chartreuse;
      `,

      class3: css`
        background-color: ${theme.roles.active.background};
      `,

      class4: css`
        background-color: ${theme.roles.active.background};
      `,
    }));

    const { current } = result;
    expect(current.class1).toEqual(current.class2);
    expect(current.class3).toEqual(current.class4);
  });

  it("Only re-evaluates the styles callback if the dependencies change during re-renders", () => {
    const { result, rerender, unmount, mockCallback } = setupUseClassNames(
      (css) => ({
        testClass: ({ color }) => css`
          color: ${color};
        `,
      }),
    );

    const initialResult = result.current.testClass;
    expect(mockCallback).toBeCalledTimes(1);

    rerender({ color: "red" });
    expect(mockCallback).toBeCalledTimes(1);
    expect(result.current.testClass).toEqual(initialResult);

    rerender({ color: "blue" });
    expect(mockCallback).toBeCalledTimes(2);
    expect(result.current).not.toEqual(initialResult);
    unmount();
  });

  it("Should use the closest theme provider available in the React tree", () => {
    const useClassNames = makeClassNames((css, theme) => ({
      testClass: css`
        background-color: ${theme.roles.active.background};
      `,
    }));

    const { result: result1 } = renderHook(useClassNames, {
      wrapper: ({ children }) => (
        <ThemeProvider theme={themes.dark}>{children}</ThemeProvider>
      ),
    });

    const { result: result2 } = renderHook(useClassNames, {
      wrapper: ({ children }) => (
        <ThemeProvider theme={themes.dark}>
          <ThemeOverride theme={themes.light}>{children}</ThemeOverride>
        </ThemeProvider>
      ),
    });

    const { result: result3 } = renderHook(useClassNames, {
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

    expect(result1.current.testClass).not.toEqual(result2.current.testClass);
    expect(result1.current.testClass).toEqual(result3.current.testClass);
  });

  it("Should recalculate all defined styles when any of the dependencies change (even if a style doesn't use them)", () => {
    const noInputsCallback = jest.fn(
      (css: typeof emotionCss) => css`
        color: yellow;
      `,
    );

    const { rerender } = setupUseClassNames((css) => ({
      withInputs: ({ color }) => css`
        background-color: ${color};
      `,

      noInputs: () => noInputsCallback(css),
    }));

    expect(noInputsCallback).toBeCalledTimes(1);
    rerender({ color: "blue" });
    expect(noInputsCallback).toBeCalledTimes(2);
  });

  it("Should calculate new styles immediately when dependencies change (no stale closure issues or delays via async tasks)", () => {
    let activeTheme: Theme = themes.dark;
    const { result, rerender, mockCallback } = setupUseClassNames(
      (css, theme) => ({
        testClass: ({ color }) => css`
          color: ${color};
          background-color: ${theme.roles.active.background};
        `,
      }),
      {
        wrapper: ({ children }) => (
          <ThemeProvider theme={activeTheme}>{children}</ThemeProvider>
        ),
      },
    );

    const initialResult = result.current.testClass;
    expect(mockCallback).toBeCalledTimes(1);

    activeTheme = themes.light;
    rerender({ color: "red" });
    expect(mockCallback).toBeCalledTimes(2);

    const themeChangeResult = result.current.testClass;
    expect(themeChangeResult).not.toEqual(initialResult);

    rerender({ color: "blue" });
    expect(mockCallback).toBeCalledTimes(3);

    const colorChangeResult = result.current.testClass;
    expect(colorChangeResult).not.toEqual(initialResult);
    expect(colorChangeResult).not.toEqual(themeChangeResult);
  });
});
