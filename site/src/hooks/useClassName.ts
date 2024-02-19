/**
 * @file This hook has had the ESLint exhaustive-deps rule added to it.
 */
import { type DependencyList, useMemo } from "react";
import { useEffectEvent } from "./hookPolyfills";
import { type Theme, useTheme } from "@emotion/react";
import { css } from "@emotion/css";

export type ClassName = (cssFn: typeof css, theme: Theme) => string;

/**
 * An escape hatch for when you really need to manually pass around a
 * `className`. Prefer using the `css` prop whenever possible. If you
 * can't use that, then this might be helpful for you.
 */
export function useClassName(styles: ClassName, deps: DependencyList): string {
  const theme = useTheme();
  const stableStylesCallback = useEffectEvent(styles);

  return useMemo(
    () => stableStylesCallback(css, theme),
    /* eslint-disable-next-line react-hooks/exhaustive-deps --
       Hook needs to be able to handle variadic number of dependencies at the
       API level. There should be a custom ESLint rule set to ensure that the
       number of dependencies doesn't change across renders. */
    [stableStylesCallback, theme, ...deps],
  );
}
