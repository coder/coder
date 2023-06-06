import { FC } from "react"
import Box from "@mui/material/Box"
import { Palette, PaletteColor } from "@mui/material/styles"
import {
  Filter,
  FilterMenu,
  MenuSkeleton,
  OptionItem,
  SearchFieldSkeleton,
  useFilter,
} from "components/Filter/filter"
import { BaseOption } from "components/Filter/options"
import { UseFilterMenuOptions, useFilterMenu } from "components/Filter/menu"

type StatusOption = BaseOption & {
  color: string
}

export const useStatusFilterMenu = ({
  value,
  onChange,
}: Pick<UseFilterMenuOptions<StatusOption>, "value" | "onChange">) => {
  const statusOptions: StatusOption[] = [
    { value: "active", label: "Active", color: "success" },
    { value: "suspended", label: "Suspended", color: "secondary" },
  ]
  return useFilterMenu({
    onChange,
    value,
    id: "status",
    getSelectedOption: async () =>
      statusOptions.find((option) => option.value === value) ?? null,
    getOptions: async () => statusOptions,
  })
}

export type StatusFilterMenu = ReturnType<typeof useStatusFilterMenu>

export const UsersFilter = ({
  filter,
  error,
  menus,
}: {
  filter: ReturnType<typeof useFilter>
  error?: unknown
  menus: {
    status: StatusFilterMenu
  }
}) => {
  return (
    <Filter
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
  )
}

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
  )
}

const StatusOptionItem = ({
  option,
  isSelected,
}: {
  option: StatusOption
  isSelected?: boolean
}) => {
  return (
    <OptionItem
      option={option}
      left={<StatusIndicator option={option} />}
      isSelected={isSelected}
    />
  )
}

const StatusIndicator: FC<{ option: StatusOption }> = ({ option }) => {
  return (
    <Box
      height={8}
      width={8}
      borderRadius={9999}
      sx={{
        backgroundColor: (theme) =>
          (theme.palette[option.color as keyof Palette] as PaletteColor).light,
      }}
    />
  )
}
