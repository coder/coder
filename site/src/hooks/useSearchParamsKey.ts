import { useSearchParams } from "react-router-dom";

export type UseSearchParamsKeyConfig = Readonly<{
  key: string;
  searchParams?: URLSearchParams;
  defaultValue?: string;
  replace?: boolean;
}>;

export type UseSearchParamKeyResult = Readonly<{
  value: string;
  onValueChange: (newValue: string) => void;
  removeValue: () => void;
}>;

export const useSearchParamsKey = (
  config: UseSearchParamsKeyConfig,
): UseSearchParamKeyResult => {
  // Cannot use function update form for setSearchParams, because by default, it
  // will always be linked to innerSearchParams, ignoring the config's params
  const [innerSearchParams, setSearchParams] = useSearchParams();

  const {
    key,
    searchParams = innerSearchParams,
    defaultValue = "",
    replace = true,
  } = config;

  return {
    value: searchParams.get(key) ?? defaultValue,
    onValueChange: (newValue) => {
      searchParams.set(key, newValue);
      setSearchParams(searchParams, { replace });
    },
    removeValue: () => {
      searchParams.delete(key);
      setSearchParams(searchParams, { replace: true });
    },
  };
};
