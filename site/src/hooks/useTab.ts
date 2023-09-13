import { useSearchParams } from "react-router-dom";

export interface UseTabResult {
  value: string;
  set: (value: string) => void;
}

export const useTab = (tabKey: string, defaultValue: string): UseTabResult => {
  const [searchParams, setSearchParams] = useSearchParams();
  const value = searchParams.get(tabKey) ?? defaultValue;

  return {
    value,
    set: (value: string) => {
      searchParams.set(tabKey, value);
      setSearchParams(searchParams, { replace: true });
    },
  };
};
