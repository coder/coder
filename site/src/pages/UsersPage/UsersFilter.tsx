import type { FC } from "react";
import {
  Filter,
  MenuSkeleton,
  SearchFieldSkeleton,
  type useFilter,
} from "components/Filter/filter";
import {
  type UseFilterMenuOptions,
  useFilterMenu,
} from "components/Filter/menu";
import {
  SelectFilter,
  type SelectFilterOption,
} from "components/Filter/SelectFilter";
import { StatusIndicator } from "components/StatusIndicator/StatusIndicator";
import { docs } from "utils/docs";

const userFilterQuery = {
  active: "status:active",
  all: "",
};

export const useStatusFilterMenu = ({
  value,
  onChange,
}: Pick<UseFilterMenuOptions<SelectFilterOption>, "value" | "onChange">) => {
  const statusOptions: SelectFilterOption[] = [
    {
      value: "active",
      label: "Active",
      startIcon: <StatusIndicator color="success" />,
    },
    {
      value: "dormant",
      label: "Dormant",
      startIcon: <StatusIndicator color="notice" />,
    },
    {
      value: "suspended",
      label: "Suspended",
      startIcon: <StatusIndicator color="warning" />,
    },
  ];
  return useFilterMenu({
    onChange,
    value,
    id: "status",
    getSelectedOption: async () =>
      statusOptions.find((option) => option.value === value) ?? null,
    getOptions: async () => statusOptions,
  });
};

export type StatusFilterMenu = ReturnType<typeof useStatusFilterMenu>;

const PRESET_FILTERS = [
  { query: userFilterQuery.active, name: "Active users" },
  { query: userFilterQuery.all, name: "All users" },
];

interface UsersFilterProps {
  filter: ReturnType<typeof useFilter>;
  error?: unknown;
  menus: {
    status: StatusFilterMenu;
  };
}

export const UsersFilter: FC<UsersFilterProps> = ({ filter, error, menus }) => {
  return (
    <Filter
      presets={PRESET_FILTERS}
      learnMoreLink={docs("/admin/users#user-filtering")}
      learnMoreLabel2="User status"
      learnMoreLink2={docs("/admin/users#user-status")}
      isLoading={menus.status.isInitializing}
      filter={filter}
      error={error}
      options={<StatusMenu {...menus.status} />}
      skeleton={
        <>
          <SearchFieldSkeleton />
          <MenuSkeleton />
        </>
      }
    />
  );
};

const StatusMenu = (menu: StatusFilterMenu) => {
  return (
    <SelectFilter
      label="Select a status"
      placeholder="All statuses"
      options={menu.searchOptions}
      onSelect={menu.selectOption}
      selectedOption={menu.selectedOption ?? undefined}
    />
  );
};
