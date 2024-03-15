import { useTheme } from "@emotion/react";
import type { FC } from "react";
import { Avatar, type AvatarProps } from "components/Avatar/Avatar";
import {
  Filter,
  FilterMenu,
  FilterSearchMenu,
  MenuSkeleton,
  OptionItem,
  SearchFieldSkeleton,
  type useFilter,
} from "components/Filter/filter";
import { type UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import { useDashboard } from "modules/dashboard/useDashboard";
import { docs } from "utils/docs";
import type { TemplateFilterMenu, StatusFilterMenu } from "./menus";
import type { TemplateOption, StatusOption } from "./options";

export const workspaceFilterQuery = {
  me: "owner:me",
  all: "",
  running: "status:running",
  failed: "status:failed",
  dormant: "dormant:true",
  outdated: "outdated:true",
};

type FilterPreset = {
  query: string;
  name: string;
};

// Can't use as const declarations to make arrays deep readonly because that
// interferes with the type contracts for Filter
const PRESET_FILTERS: FilterPreset[] = [
  {
    query: workspaceFilterQuery.me,
    name: "My workspaces",
  },
  {
    query: workspaceFilterQuery.all,
    name: "All workspaces",
  },
  {
    query: workspaceFilterQuery.running,
    name: "Running workspaces",
  },
  {
    query: workspaceFilterQuery.failed,
    name: "Failed workspaces",
  },
  {
    query: workspaceFilterQuery.outdated,
    name: "Outdated workspaces",
  },
];

// Defined outside component so that the array doesn't get reconstructed each render
const PRESETS_WITH_DORMANT: FilterPreset[] = [
  ...PRESET_FILTERS,
  {
    query: workspaceFilterQuery.dormant,
    name: "Dormant workspaces",
  },
];

type WorkspaceFilterProps = {
  filter: ReturnType<typeof useFilter>;
  error?: unknown;
  menus: {
    user?: UserFilterMenu;
    template: TemplateFilterMenu;
    status: StatusFilterMenu;
  };
};

export const WorkspacesFilter: FC<WorkspaceFilterProps> = ({
  filter,
  error,
  menus,
}) => {
  const { entitlements } = useDashboard();
  const presets = entitlements.features["advanced_template_scheduling"].enabled
    ? PRESETS_WITH_DORMANT
    : PRESET_FILTERS;

  return (
    <Filter
      presets={presets}
      isLoading={menus.status.isInitializing}
      filter={filter}
      error={error}
      learnMoreLink={docs("/workspaces#workspace-filtering")}
      options={
        <>
          {menus.user && <UserMenu menu={menus.user} />}
          <TemplateMenu {...menus.template} />
          <StatusMenu {...menus.status} />
        </>
      }
      skeleton={
        <>
          <SearchFieldSkeleton />
          {menus.user && <MenuSkeleton />}
          <MenuSkeleton />
          <MenuSkeleton />
        </>
      }
    />
  );
};

const TemplateMenu = (menu: TemplateFilterMenu) => {
  return (
    <FilterSearchMenu
      id="templates-menu"
      menu={menu}
      label={
        menu.selectedOption ? (
          <TemplateOptionItem option={menu.selectedOption} />
        ) : (
          "All templates"
        )
      }
    >
      {(itemProps) => <TemplateOptionItem {...itemProps} />}
    </FilterSearchMenu>
  );
};

interface TemplateOptionItemProps {
  option: TemplateOption;
  isSelected?: boolean;
}

const TemplateOptionItem: FC<TemplateOptionItemProps> = ({
  option,
  isSelected,
}) => {
  return (
    <OptionItem
      option={option}
      isSelected={isSelected}
      left={
        <TemplateAvatar
          templateName={option.label}
          icon={option.icon}
          css={{ width: 14, height: 14, fontSize: 8 }}
        />
      }
    />
  );
};

interface TemplateAvatarProps extends AvatarProps {
  templateName: string;
  icon?: string;
}

const TemplateAvatar: FC<TemplateAvatarProps> = ({
  templateName,
  icon,
  ...avatarProps
}) => {
  return icon ? (
    <Avatar src={icon} variant="square" fitImage {...avatarProps} />
  ) : (
    <Avatar {...avatarProps}>{templateName}</Avatar>
  );
};

const StatusMenu = (menu: StatusFilterMenu) => {
  return (
    <FilterMenu
      id="status-menu"
      menu={menu}
      label={
        menu.selectedOption ? (
          <StatusOptionItem option={menu.selectedOption} />
        ) : (
          "All statuses"
        )
      }
    >
      {(itemProps) => <StatusOptionItem {...itemProps} />}
    </FilterMenu>
  );
};

interface StatusOptionItem {
  option: StatusOption;
  isSelected?: boolean;
}

const StatusOptionItem: FC<StatusOptionItem> = ({ option, isSelected }) => {
  return (
    <OptionItem
      option={option}
      left={<StatusIndicator option={option} />}
      isSelected={isSelected}
    />
  );
};

interface StatusIndicatorProps {
  option: StatusOption;
}

const StatusIndicator: FC<StatusIndicatorProps> = ({ option }) => {
  const theme = useTheme();

  return (
    <div
      css={{
        height: 8,
        width: 8,
        borderRadius: 4,
        backgroundColor: theme.roles[option.color].fill.solid,
      }}
    />
  );
};
