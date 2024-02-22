import { type Theme, useTheme } from "@emotion/react";
import { css } from "@emotion/css";
import { useState } from "react";

type Primitive = string | number | boolean | null | undefined | symbol | bigint;
type EmptyObject = Record<string, never>;

type CSSInput = Readonly<{
  css: typeof css;
  theme: Theme;
}>;

type ClassNameFunction<TInput extends NonNullable<unknown>> = (
  args: CSSInput & TInput,
) => string;

type MakeClassNamesResult<
  THookInput extends Record<string, Primitive>,
  TConfig extends Record<string, ClassNameFunction<THookInput>>,
> = (hookInput: THookInput) => Readonly<Record<keyof TConfig, string>>;

/**
 * Hook factory for giving you an escape hatch for making Emotion styles. This
 * should be used as a last resort; use the React CSS prop whenever possible.
 *
 * ---
 *
 * Sometimes you need to combine/collate styles using the className prop in a
 * component, but Emotion does not give you an easy way to define a className
 * and use it from within the same component.
 *
 * Other times, you need to use inputs that will change on each render to make
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
>(styleConfig: TConfig): MakeClassNamesResult<THookInput, TConfig> {
  type StyleRecord = Record<keyof TConfig, string>;

  const computeNewStyles = (theme: Theme, hookInput: THookInput) => {
    const result: Partial<StyleRecord> = {};
    for (const key in styleConfig) {
      const configFunc = styleConfig[key];
      result[key] = configFunc({ css, theme, ...hookInput });
    }

    return result as Readonly<StyleRecord>;
  };

  const didInputsChangeByValue = (
    inputs1: THookInput,
    inputs2: THookInput,
  ): boolean => {
    for (const key in inputs1) {
      const value1 = inputs1[key];
      const value2 = inputs2[key];

      if (Number.isNaN(value1) && Number.isNaN(value2)) {
        continue;
      }

      if (value1 !== value2) {
        return true;
      }
    }

    return false;
  };

  return function useClassNames(hookInputs) {
    const activeTheme = useTheme();
    const computeNewCacheValue = () => ({
      theme: activeTheme,
      inputs: hookInputs,
      styles: computeNewStyles(activeTheme, hookInputs),
    });

    const [cache, setCache] = useState(computeNewCacheValue);
    const needNewStyles =
      cache.theme !== activeTheme ||
      didInputsChangeByValue(cache.inputs, hookInputs);

    if (needNewStyles) {
      setCache(computeNewCacheValue());
    }

    return cache.styles;
  };
}

/**
 * Issues left to figure out:
 * 1. Bare minimum, you'll almost always want to pass in one type parameter for
 *    the additional inputs that you're accessing. Having one explicit type
 *    parameter should not break type inference for the other type parameter,
 *    and destroy auto-complete for classNames's properties
 * 2. If the hook is being called with no additional inputs, it'd be nice if you
 *    could just call the hook with no arguments whatsoever
 */
type HookInput = Readonly<{
  paddingTop: number;
  variant: "contained" | "stroked";
}>;

const useClassNames = makeClassNames<HookInput>({
  class1: ({ css, theme, paddingTop }) => css`
    background-color: red;
    padding: ${theme.spacing(2)};
    padding-top: ${paddingTop}px;
  `,

  class2: ({ css, variant }) => css`
    color: ${variant === "contained" ? "red" : "blue"};
  `,
});

/**
 * Idea I have - there are two main benefits to having the main function
 * argument be defined like this:
 * 1. Gives you another function boundary, so TConfig should hopefully be
 *    bindable to it. That lets you split up the type parameters and gives you
 *    more options for restoring type inference/auto-complete
 * 2. The logic centralizes the css and theme arguments, which reduces keyboard
 *    typing
 */
// const useRevampedClassNames = makeClassNames<HookInput>((css, theme) => ({
//   class1: ({ paddingTop }) => css`
//     background-color: red;
//     padding: ${theme.spacing(2)};
//     padding-top: ${paddingTop}px;
//   `,

//   class2: ({ variant }) => css`
//     color: ${variant === "contained" ? "red" : "blue"};
//   `,
// }));

// function makeClassNames2(styleFunction) {
//   const computeNewStyles = (theme, hookInput) => {
//     const stylesObject = styleFunction(css, theme);
//     const result = {};

//     for (const key in stylesObject) {
//       const configFunc = stylesObject[key];
//       result[key] = configFunc(hookInput);
//     }

//     return result;
//   };

//   const didInputsChangeByValue = (inputs1, inputs2) => {
//     for (const key in inputs1) {
//       const value1 = inputs1[key];
//       const value2 = inputs2[key];

//       if (Number.isNaN(value1) && Number.isNaN(value2)) {
//         continue;
//       }

//       if (value1 !== value2) {
//         return true;
//       }
//     }

//     return false;
//   };

//   return function useClassNames(hookInput) {
//     const activeTheme = useTheme();
//     const computeNewCacheValue = () => ({
//       prevTheme: activeTheme,
//       prevInput: hookInput,
//       styles: computeNewStyles(activeTheme, hookInput),
//     });

//     const [cache, setCache] = useState(computeNewCacheValue);
//     const needNewStyles =
//       cache.prevTheme !== activeTheme ||
//       didInputsChangeByValue(cache.prevInput, hookInput);

//     if (needNewStyles) {
//       setCache(computeNewCacheValue());
//     }

//     return cache.styles;
//   };
// }

export function useTempBlah() {
  const classNames = useClassNames({ variant: "contained", paddingTop: 12 });
  return classNames;
}
