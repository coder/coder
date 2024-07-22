import capitalize from "lodash/capitalize";
import type { FC } from "react";
import { API } from "api/api";
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
  SelectFilterSearch,
  type SelectFilterOption,
} from "components/Filter/SelectFilter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import { UserAvatar } from "components/UserAvatar/UserAvatar";
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
    organization: OrganizationsFilterMenu;
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
          <OrganizationsMenu menu={menus.organization} />
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

export const useOrganizationsFilterMenu = ({
  value,
  onChange,
}: Pick<UseFilterMenuOptions<SelectFilterOption>, "value" | "onChange">) => {
  return useFilterMenu({
    onChange,
    value,
    id: "organizations",
    getSelectedOption: async () => {
      if (value) {
        const organizations = await API.getOrganizations();
        const organization = organizations.find((o) => o.name === value);
        if (organization) {
          return {
            label: organization.display_name || organization.name,
            value: organization.name,
            startIcon: (
              <UserAvatar
                key={organization.id}
                size="xs"
                username={organization.display_name || organization.name}
                avatarURL={organization.icon}
              />
            ),
          };
        }
      }
      return null;
    },
    getOptions: async () => {
      const organizationsRes = await API.getOrganizations();
      return organizationsRes.map<SelectFilterOption>((organization) => ({
        label: organization.display_name || organization.name,
        value: organization.name,
        startIcon: (
          <UserAvatar
            key={organization.id}
            size="xs"
            username={organization.display_name || organization.name}
            avatarURL={organization.icon}
          />
        ),
      }));
    },
  });
};

export type OrganizationsFilterMenu = ReturnType<
  typeof useOrganizationsFilterMenu
>;

interface OrganizationsMenuProps {
  menu: OrganizationsFilterMenu;
}

export const OrganizationsMenu: FC<OrganizationsMenuProps> = ({ menu }) => {
  return (
    <SelectFilter
      label="Select an organization"
      placeholder="All organizations"
      emptyText="No organizations found"
      options={menu.searchOptions}
      onSelect={menu.selectOption}
      selectedOption={menu.selectedOption ?? undefined}
      selectFilterSearch={
        <SelectFilterSearch
          inputProps={{ "aria-label": "Search organization" }}
          placeholder="Search organization..."
          value={menu.query}
          onChange={menu.setQuery}
        />
      }
    />
  );
};
