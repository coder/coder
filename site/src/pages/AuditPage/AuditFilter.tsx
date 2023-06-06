import { AuditActions } from "api/typesGenerated"
import { UserFilterMenu, UserMenu } from "components/Filter/UserFilter"
import {
  Filter,
  FilterMenu,
  MenuSkeleton,
  OptionItem,
  SearchFieldSkeleton,
  useFilter,
} from "components/Filter/filter"
import { UseFilterMenuOptions, useFilterMenu } from "components/Filter/menu"
import { BaseOption } from "components/Filter/options"
import capitalize from "lodash/capitalize"

export const AuditFilter = ({
  filter,
  error,
  menus,
}: {
  filter: ReturnType<typeof useFilter>
  error?: unknown
  menus: {
    user: UserFilterMenu
    action: ActionFilterMenu
  }
}) => {
  return (
    <Filter
      isLoading={menus.user.isInitializing}
      filter={filter}
      error={error}
      options={
        <>
          <ActionMenu {...menus.action} />
          <UserMenu {...menus.user} />
        </>
      }
      skeleton={
        <>
          <SearchFieldSkeleton />
          <MenuSkeleton />
          <MenuSkeleton />
        </>
      }
    />
  )
}

export const useActionFilterMenu = ({
  value,
  onChange,
}: Pick<UseFilterMenuOptions<BaseOption>, "value" | "onChange">) => {
  const actionOptions: BaseOption[] = AuditActions.map((action) => ({
    value: action,
    label: capitalize(action),
  }))
  return useFilterMenu({
    onChange,
    value,
    id: "status",
    getSelectedOption: async () =>
      actionOptions.find((option) => option.value === value) ?? null,
    getOptions: async () => actionOptions,
  })
}

export type ActionFilterMenu = ReturnType<typeof useActionFilterMenu>

const ActionMenu = (menu: ActionFilterMenu) => {
  return (
    <FilterMenu
      id="action-menu"
      menu={menu}
      label={
        menu.selectedOption ? (
          <OptionItem option={menu.selectedOption} />
        ) : (
          "All actions"
        )
      }
    >
      {(itemProps) => <OptionItem {...itemProps} />}
    </FilterMenu>
  )
}
