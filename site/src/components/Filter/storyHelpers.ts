import { action } from "@storybook/addon-actions";
import { UseFilterResult } from "./filter";
import { UseFilterMenuResult } from "./menu";

export const MockMenu: UseFilterMenuResult = {
  initialOption: undefined,
  isInitializing: false,
  isSearching: false,
  query: "",
  searchOptions: [],
  selectedOption: undefined,
  selectOption: action("selectOption"),
  setQuery: action("updateQuery"),
};

export const getDefaultFilterProps = <TFilterProps>({
  query = "",
  values,
  menus,
}: {
  query?: string;
  values: Record<string, string | undefined>;
  menus: Record<string, UseFilterMenuResult>;
}) =>
  ({
    filter: {
      query,
      update: () => action("update"),
      debounceUpdate: action("debounce") as UseFilterResult["debounceUpdate"],
      used: false,
      values,
    },
    menus,
  }) as TFilterProps;
