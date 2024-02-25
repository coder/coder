import { useReducer } from "react";
import { type Theme, useTheme } from "@emotion/react";
import { css as emotionCss } from "@emotion/css";

type Primitive = string | number | boolean | null | undefined | symbol | bigint;
type PrimitiveRecord = Record<string, Primitive>;
type EmptyRecord = Record<string, never>;

type MapRawInputToHookInput<TRawInput extends PrimitiveRecord> = [
  TRawInput,
] extends [never]
  ? EmptyRecord | null
  : TRawInput;

type ClassNameFunction<T extends PrimitiveRecord> = (
  input: MapRawInputToHookInput<T>,
) => string;

export type StylesFunction<
  TKey extends string,
  TRawHookInput extends PrimitiveRecord,
> = (
  css: typeof emotionCss,
  theme: Theme,
) => Record<TKey, string | ClassNameFunction<TRawHookInput>>;

type UseClassNamesCustomHookResult<TKey extends string> = Record<TKey, string>;

type UseClassNamesCustomHook<
  TKey extends string,
  TRawHookInput extends PrimitiveRecord,
> = (
  input: MapRawInputToHookInput<TRawHookInput>,
) => Readonly<UseClassNamesCustomHookResult<TKey>>;

/**
 * Hook factory for giving you an escape hatch for making Emotion styles. This
 * should be used as a last resort; use the React CSS prop whenever possible.
 *
 * ---
 *
 * Sometimes you need to combine/collate styles using the `className` prop in a
 * component, but Emotion does not give you an easy way to define a className
 * and use it from within the same component.
 *
 * Other times, you need to use inputs that will change on each render to make
 * your styles, but you only want the styles relying on those inputs to be
 * re-computed when the input actually changes. Otherwise, Emotion's CSS
 * function might keep thrashing the <style> tag with diffs each and every
 * render.
 *
 * And sometimes just you don't want to think about dependency arrays and
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
  /*
     Making the user specify the keys first is a heavy-handed way to circumvent
     TypeScript language limitations and make sure type safety doesn't degrade.

     TS type inference is all or nothing; if you specify one type parameter,
     type inference stops and default values kick in for all other parameters

     If the input were specified first, a dev could accidentally specify only
     the input. That would cause the key to get inferred as type string, and
     they would lose auto-complete and protection against accidental typos.
     Ergonomics are slightly worse this way, but that should be fine for what'
     a last-resort escape hatch anyway

     Could maybe get cleaned up if private type parameters ever become a thing
  */
  TKey extends string = string,
  TRawHookInput extends PrimitiveRecord = never,
>(
  styleFunction: StylesFunction<TKey, TRawHookInput>,
): UseClassNamesCustomHook<TKey, TRawHookInput> {
  type Result = UseClassNamesCustomHookResult<TKey>;
  type HookInput = MapRawInputToHookInput<TRawHookInput>;

  const computeNewStyles = (
    theme: Theme,
    hookInput: HookInput,
  ): Readonly<Result> => {
    const stylesObject = styleFunction(emotionCss, theme);
    const result: Partial<Result> = {};

    for (const key in stylesObject) {
      const styleValue = stylesObject[key];
      const resultValue =
        typeof styleValue === "string" ? styleValue : styleValue(hookInput);

      result[key] = resultValue;
    }

    return result as Result;
  };

  // This function could technically be defined outside, since it doesn't rely
  // on closure, but feeding in the right type info got too awkward
  const didInputsChangeByValue = (
    hookInput1: HookInput,
    hookInput2: HookInput,
  ): boolean => {
    // Slightly clunky syntax, but have to do it this way to make the compiler
    // happy and not make it worry about null values in the loop
    if (hookInput1 === null) {
      if (hookInput2 === null) {
        return false;
      }

      return true;
    } else if (hookInput2 === null) {
      return true;
    }

    for (const key in hookInput1) {
      const value1 = hookInput1[key];
      const value2 = hookInput2[key];

      if (Number.isNaN(value1) && Number.isNaN(value2)) {
        continue;
      }

      if (value1 !== value2) {
        return true;
      }
    }

    return false;
  };

  return function useClassNames(hookInput) {
    const activeTheme = useTheme();
    const getNewCacheValue = () => ({
      prevTheme: activeTheme,
      prevInput: hookInput,
      styles: computeNewStyles(activeTheme, hookInput),
    });

    const [cache, updateCacheAndRedoRender] = useReducer(
      getNewCacheValue,
      null,
      getNewCacheValue,
    );

    const needNewStyles =
      hookInput !== null &&
      (cache.prevTheme !== activeTheme ||
        didInputsChangeByValue(cache.prevInput, hookInput));

    if (needNewStyles) {
      updateCacheAndRedoRender();
    }

    return cache.styles;
  };
}
