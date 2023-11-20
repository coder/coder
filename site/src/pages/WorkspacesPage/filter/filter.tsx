import { FC } from "react";
import Box from "@mui/material/Box";
import { useIsWorkspaceActionsEnabled } from "components/Dashboard/DashboardProvider";
import { Avatar, AvatarProps } from "components/Avatar/Avatar";
import { Palette, PaletteColor } from "@mui/material/styles";
import { TemplateFilterMenu, StatusFilterMenu } from "./menus";
import { TemplateOption, StatusOption } from "./options";
import {
  Filter,
  FilterMenu,
  FilterSearchMenu,
  MenuSkeleton,
  OptionItem,
  SearchFieldSkeleton,
  useFilter,
} from "components/Filter/filter";
import { UserFilterMenu, UserMenu } from "components/Filter/UserFilter";
import { docs } from "utils/docs";

export const workspaceFilterQuery = {
  me: "owner:me",
  all: "",
  running: "status:running",
  failed: "status:failed",
  dormant: "is-dormant:true",
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

export const WorkspacesFilter = ({
  filter,
  error,
  menus,
}: WorkspaceFilterProps) => {
  const actionsEnabled = useIsWorkspaceActionsEnabled();
  const presets = actionsEnabled ? PRESETS_WITH_DORMANT : PRESET_FILTERS;

  return (
    <Filter
      presets={presets}
      isLoading={menus.status.isInitializing}
      filter={filter}
      error={error}
      learnMoreLink={docs("/workspaces#workspace-filtering")}
      options={
        <>
          {menus.user && <UserMenu {...menus.user} />}
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

const TemplateOptionItem = ({
  option,
  isSelected,
}: {
  option: TemplateOption;
  isSelected?: boolean;
}) => {
  return (
    <OptionItem
      option={option}
      isSelected={isSelected}
      left={
        <TemplateAvatar
          templateName={option.label}
          icon={option.icon}
          sx={{ width: 14, height: 14, fontSize: 8 }}
        />
      }
    />
  );
};

const TemplateAvatar: FC<
  AvatarProps & { templateName: string; icon?: string }
> = ({ templateName, icon, ...avatarProps }) => {
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

const StatusOptionItem = ({
  option,
  isSelected,
}: {
  option: StatusOption;
  isSelected?: boolean;
}) => {
  return (
    <OptionItem
      option={option}
      left={<StatusIndicator option={option} />}
      isSelected={isSelected}
    />
  );
};

const StatusIndicator: FC<{ option: StatusOption }> = ({ option }) => {
  const color = option.color === "notice" ? "warning" : option.color;

  return (
    <Box
      height={8}
      width={8}
      borderRadius={9999}
      css={(theme) => {
        return {
          backgroundColor: (
            theme.palette[color as keyof Palette] as PaletteColor
          ).light,
        };
      }}
    />
  );
};
