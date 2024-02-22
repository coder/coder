import { type Theme, css, useTheme } from "@emotion/react";
import { useState } from "react";

type Primitive = string | number | boolean | null | undefined | symbol | bigint;
type EmptyObject = Record<string, never>;

type CSSInput = Readonly<{
  css: typeof css;
  theme: Theme;
}>;

type ClassNameFunction<TInput extends NonNullable<unknown>> = (
  args: CSSInput & TInput,
) => string; // TEMP!

/**
 * Hook factory for giving you an escape hatch for making Emotion styles.
 *
 * Sometimes you need to combine/collate styles using the className prop in a
 * component, but Emotion does not give you an easy way to define a className
 * and use it from within the same component.
 *
 * Other times, you need to use  inputs that will change on each render to make
 * your styles, but you only want the styles relying on those inputs to be
 * re-computed when the input actually changes. Otherwise, CSS will keep
 * thrashing the <style> tag with diffs each and every render.
 *
 * Also, sometimes just you don't want to think about dependency arrays and
 * stale closure issues.
 *
 * This function solves all three problems. The custom hook it returns will give
 * you a type-safe collection of className values, and auto-memoize all inputs,
 * only re-computing new CSS styles when one of the inputs changes by reference.
 *
 * Making that memoization possible comes with two caveats:
 * 1. All inputs fed into the hook must be primitives. No nested objects or
 *    functions or arrays.
 * 2. All styles defined via the hook are tied to the same memoization cache. If
 *    one of the inputs changes, all classnames for the hook will be
 *    re-computed, even if none of the classnames actually use the input that
 *    changed.
 *
 * If (2) is a performance problem, you can define separate hooks by calling
 * makeClassNames multiple times for each hook you need.
 */
export function makeClassNames<
  THookInput extends Record<string, Primitive> = EmptyObject,
  TConfig extends Record<string, ClassNameFunction<THookInput>> = Record<
    string,
    ClassNameFunction<THookInput>
  >,
>(
  styleConfig: TConfig,
): (hookInput: THookInput) => Record<keyof TConfig, string> {
  type StyleRecord = Record<keyof TConfig, string>;

  const computeNewStyles = (
    theme: Theme,
    hookProps: THookInput,
  ): StyleRecord => {
    const result: Partial<StyleRecord> = {};

    for (const key in styleConfig) {
      const configFunc = styleConfig[key];
      result[key] = configFunc({ css, theme, ...hookProps });
    }

    return result as StyleRecord;
  };

  const didPropsChangeByValue = (
    hookProps1: THookInput,
    hookProps2: THookInput,
  ): boolean => {
    for (const key in hookProps1) {
      const prop1 = hookProps1[key];
      const prop2 = hookProps2[key];

      if (Number.isNaN(prop1) && Number.isNaN(prop2)) {
        continue;
      }

      if (prop1 !== prop2) {
        return true;
      }
    }

    return false;
  };

  return function useClassNames(hookProps: THookInput): StyleRecord {
    const activeTheme = useTheme();
    const computeNewCache = () => {
      return {
        theme: activeTheme,
        props: hookProps,
        styles: computeNewStyles(activeTheme, hookProps),
      };
    };

    const [cache, setCache] = useState(computeNewCache);
    const needNewStyles =
      cache.theme !== activeTheme ||
      didPropsChangeByValue(cache.props, hookProps);

    if (needNewStyles) {
      setCache(computeNewCache());
    }

    return cache.styles;
  };
}

// type HookProps = Readonly<{ arg1: string; arg2: string }>;

// const useClassnames = makeClassNames<HookProps>({
//   class1: ({ css, theme, arg1 }) => css`
//     background-color: red;
//   `,

//   class2: ({ css, theme, arg2 }) => css``,
// });
