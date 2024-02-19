import { useCallback } from "react";
import { useSearchParams } from "react-router-dom";
import { useEffectEvent } from "./hookPolyfills";

export type UseSearchParamKeyConfig = Readonly<{
  defaultValue?: string;
  replace?: boolean;
}>;

export type UseSearchParamKeyResult = Readonly<{
  value: string;
  onValueChange: (newValue: string) => void;
  removeValue: () => void;
}>;

export const useSearchParamsKey = (
  key: string,
  config: UseSearchParamKeyConfig = {},
): UseSearchParamKeyResult => {
  const { defaultValue = "", replace = true } = config;
  const [searchParams, setSearchParams] = useSearchParams();
  const stableSetSearchParams = useEffectEvent(setSearchParams);

  const onValueChange = useCallback(
    (newValue: string) => {
      stableSetSearchParams(
        (currentParams) => {
          currentParams.set(key, newValue);
          return currentParams;
        },
        { replace },
      );
    },
    [stableSetSearchParams, key, replace],
  );

  const removeValue = useCallback(() => {
    stableSetSearchParams(
      (currentParams) => {
        currentParams.delete(key);
        return currentParams;
      },
      { replace },
    );
  }, [stableSetSearchParams, key, replace]);

  return {
    value: searchParams.get(key) ?? defaultValue,
    onValueChange,
    removeValue,
  };
};
