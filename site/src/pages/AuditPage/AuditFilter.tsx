import capitalize from "lodash/capitalize";
import type { FC } from "react";
import { AuditActions, ResourceTypes } from "api/typesGenerated";
import {
  Filter,
  MenuSkeleton,
  SearchFieldSkeleton,
  type useFilter,
} from "components/Filter/filter";
import {
  useFilterMenu,
  type UseFilterMenuOptions,
} from "components/Filter/menu";
import {
  SelectFilter,
  type SelectFilterOption,
} from "components/Filter/SelectFilter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import { docs } from "utils/docs";

const PRESET_FILTERS = [
  {
    query: "resource_type:workspace action:create",
    name: "Created workspaces",
  },
  { query: "resource_type:template action:create", name: "Added templates" },
  { query: "resource_type:user action:delete", name: "Deleted users" },
  {
    query: "resource_type:workspace_build action:start build_reason:initiator",
    name: "Builds started by a user",
  },
  {
    query: "resource_type:api_key action:login",
    name: "User logins",
  },
];

interface AuditFilterProps {
  filter: ReturnType<typeof useFilter>;
  error?: unknown;
  menus: {
    user: UserFilterMenu;
    action: ActionFilterMenu;
    resourceType: ResourceTypeFilterMenu;
  };
}

export const AuditFilter: FC<AuditFilterProps> = ({ filter, error, menus }) => {
  return (
    <Filter
      learnMoreLink={docs("/admin/audit-logs#filtering-logs")}
      presets={PRESET_FILTERS}
      isLoading={menus.user.isInitializing}
      filter={filter}
      error={error}
      options={
        <>
          <ResourceTypeMenu {...menus.resourceType} />
          <ActionMenu {...menus.action} />
          <UserMenu menu={menus.user} />
        </>
      }
      skeleton={
        <>
          <SearchFieldSkeleton />
          <MenuSkeleton />
          <MenuSkeleton />
          <MenuSkeleton />
        </>
      }
    />
  );
};

export const useActionFilterMenu = ({
  value,
  onChange,
}: Pick<UseFilterMenuOptions<SelectFilterOption>, "value" | "onChange">) => {
  const actionOptions: SelectFilterOption[] = AuditActions.map((action) => ({
    value: action,
    label: capitalize(action),
  }));
  return useFilterMenu({
    onChange,
    value,
    id: "status",
    getSelectedOption: async () =>
      actionOptions.find((option) => option.value === value) ?? null,
    getOptions: async () => actionOptions,
  });
};

export type ActionFilterMenu = ReturnType<typeof useActionFilterMenu>;

const ActionMenu = (menu: ActionFilterMenu) => {
  return (
    <SelectFilter
      label="Select an action"
      placeholder="All actions"
      options={menu.searchOptions}
      onSelect={menu.selectOption}
      selectedOption={menu.selectedOption ?? undefined}
    />
  );
};

export const useResourceTypeFilterMenu = ({
  value,
  onChange,
}: Pick<UseFilterMenuOptions<SelectFilterOption>, "value" | "onChange">) => {
  const actionOptions: SelectFilterOption[] = ResourceTypes.map((type) => {
    let label = capitalize(type);

    if (type === "api_key") {
      label = "API Key";
    }

    if (type === "git_ssh_key") {
      label = "Git SSH Key";
    }

    if (type === "template_version") {
      label = "Template Version";
    }

    if (type === "workspace_build") {
      label = "Workspace Build";
    }

    return {
      value: type,
      label,
    };
  });
  return useFilterMenu({
    onChange,
    value,
    id: "resourceType",
    getSelectedOption: async () =>
      actionOptions.find((option) => option.value === value) ?? null,
    getOptions: async () => actionOptions,
  });
};

export type ResourceTypeFilterMenu = ReturnType<
  typeof useResourceTypeFilterMenu
>;

const ResourceTypeMenu = (menu: ResourceTypeFilterMenu) => {
  return (
    <SelectFilter
      label="Select a resource type"
      placeholder="All resource types"
      options={menu.searchOptions}
      onSelect={menu.selectOption}
      selectedOption={menu.selectedOption ?? undefined}
    />
  );
};
