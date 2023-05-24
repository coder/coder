import { WorkspaceStatuses } from "api/typesGenerated"
import { FC, ReactNode, forwardRef, useMemo, useRef, useState } from "react"
import Box from "@mui/material/Box"
import TextField from "@mui/material/TextField"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown"
import Button, { ButtonProps } from "@mui/material/Button"
import Menu, { MenuProps } from "@mui/material/Menu"
import MenuItem from "@mui/material/MenuItem"
import SearchOutlined from "@mui/icons-material/SearchOutlined"
import { Avatar, AvatarProps } from "components/Avatar/Avatar"
import InputAdornment from "@mui/material/InputAdornment"
import { Palette, PaletteColor } from "@mui/material/styles"
import IconButton from "@mui/material/IconButton"
import Tooltip from "@mui/material/Tooltip"
import CloseOutlined from "@mui/icons-material/CloseOutlined"
import { getDisplayWorkspaceStatus } from "utils/workspace"
import { Loader } from "components/Loader/Loader"
import MenuList from "@mui/material/MenuList"
import { useSearchParams } from "react-router-dom"
import { useQuery } from "@tanstack/react-query"
import { getUsers, getTemplates } from "api/api"
import Skeleton, { SkeletonProps } from "@mui/material/Skeleton"

/** Filter */

export type FilterValues = {
  owner?: string // User["username"]
  status?: string // WorkspaceStatus
  template?: string // Template["name"]
}

export const useFilter = () => {
  const [searchParams, setSearchParams] = useSearchParams()
  const query = searchParams.get("filter") ?? ""
  const values = parseFilterQuery(query)

  const update = (values: string | FilterValues) => {
    if (typeof values === "string") {
      searchParams.set("filter", values)
    } else {
      searchParams.set("filter", stringifyFilter(values))
    }
    setSearchParams(searchParams)
  }

  return {
    query,
    update,
    values,
  }
}

type UseFilterResult = ReturnType<typeof useFilter>

const parseFilterQuery = (filterQuery: string): FilterValues => {
  if (filterQuery === "") {
    return {}
  }

  const pairs = filterQuery.split(" ")
  const result: FilterValues = {}

  for (const pair of pairs) {
    const [key, value] = pair.split(":") as [
      keyof FilterValues,
      string | undefined,
    ]
    if (value) {
      result[key] = value
    }
  }

  return result
}

const stringifyFilter = (filterValue: FilterValues): string => {
  let result = ""

  for (const key in filterValue) {
    const value = filterValue[key as keyof FilterValues]
    if (value) {
      result += `${key}:${value} `
    }
  }

  return result.trim()
}

/** Autocomplete */

type BaseOption = {
  label: string
  value: string
}

type OwnerOption = BaseOption & {
  avatarUrl?: string
}

type StatusOption = BaseOption & {
  color: string
}

type TemplateOption = BaseOption & {
  icon?: string
}

type UseAutocompleteOptions<TOption extends BaseOption> = {
  id: string
  initialQuery?: string
  // Using null because of react-query
  // https://tanstack.com/query/v4/docs/react/guides/migrating-to-react-query-4#undefined-is-an-illegal-cache-value-for-successful-queries
  getInitialOption: () => Promise<TOption | null>
  getOptions: (query: string) => Promise<TOption[]>
  onChange: (option: TOption | undefined) => void
}

const useAutocomplete = <TOption extends BaseOption = BaseOption>({
  id,
  getInitialOption,
  getOptions,
  onChange,
}: UseAutocompleteOptions<TOption>) => {
  const [query, setQuery] = useState("")
  const [selectedOption, setSelectedOption] = useState<TOption>()
  const initialOptionQuery = useQuery({
    queryKey: [id, "autocomplete", "initial"],
    queryFn: () => getInitialOption(),
    onSuccess: (option) => setSelectedOption(option ?? undefined),
  })
  const searchOptionsQuery = useQuery({
    queryKey: [id, "autoComplete", "search"],
    queryFn: () => getOptions(query),
  })
  const searchOptions = useMemo(() => {
    const isDataLoaded =
      searchOptionsQuery.isFetched && initialOptionQuery.isFetched

    if (!isDataLoaded) {
      return undefined
    }

    let options = searchOptionsQuery.data as TOption[]

    if (!selectedOption) {
      return options
    }

    // We will add the initial option on the top of the options
    // 1 - remove the initial option from the search options if it exists
    // 2 - add the initial option on the top
    options = options.filter((option) => option.value !== selectedOption.value)
    options.unshift(selectedOption)

    // Filter data based o search query
    options = options.filter(
      (option) =>
        option.label.toLowerCase().includes(query.toLowerCase()) ||
        option.value.toLowerCase().includes(query.toLowerCase()),
    )

    return options
  }, [
    initialOptionQuery.isFetched,
    query,
    searchOptionsQuery.data,
    searchOptionsQuery.isFetched,
    selectedOption,
  ])

  const selectOption = (option: TOption) => {
    let newSelectedOptionValue: TOption | undefined = option

    if (option.value === selectedOption?.value) {
      newSelectedOptionValue = undefined
    }

    if (onChange) {
      onChange(newSelectedOptionValue)
    }
    setSelectedOption(newSelectedOptionValue)
  }
  const clearSelection = () => {
    setSelectedOption(undefined)
  }

  return {
    query,
    setQuery,
    selectedOption,
    selectOption,
    clearSelection,
    isInitializing: initialOptionQuery.isInitialLoading,
    initialOption: initialOptionQuery.data,
    isSearching: searchOptionsQuery.isFetching,
    searchOptions,
  }
}

export const useUsersAutocomplete = (
  initialOptionValue: string | undefined,
  onChange: (option: OwnerOption | undefined) => void,
) =>
  useAutocomplete({
    id: "owner",
    getInitialOption: async () => {
      const usersRes = await getUsers({ q: initialOptionValue, limit: 1 })
      const firstUser = usersRes.users.at(0)
      if (firstUser && firstUser.username === initialOptionValue) {
        return {
          label: firstUser.username,
          value: firstUser.username,
          avatarUrl: firstUser.avatar_url,
        }
      }
      return null
    },
    getOptions: async (query) => {
      const usersRes = await getUsers({ q: query, limit: 25 })
      return usersRes.users.map((user) => ({
        label: user.username,
        value: user.username,
        avatarUrl: user.avatar_url,
      }))
    },
    onChange,
  })

type UsersAutocomplete = ReturnType<typeof useUsersAutocomplete>

export const useTemplatesAutocomplete = (
  orgId: string,
  initialOptionValue: string | undefined,
  onChange: (option: TemplateOption | undefined) => void,
) => {
  return useAutocomplete({
    id: "template",
    getInitialOption: async () => {
      const templates = await getTemplates(orgId)
      const template = templates.find(
        (template) => template.name === initialOptionValue,
      )
      if (template) {
        return {
          label:
            template.display_name !== ""
              ? template.display_name
              : template.name,
          value: template.name,
          icon: template.icon,
        }
      }
      return null
    },
    getOptions: async (query) => {
      const templates = await getTemplates(orgId)
      const filteredTemplates = templates.filter(
        (template) =>
          template.name.toLowerCase().includes(query.toLowerCase()) ||
          template.display_name.toLowerCase().includes(query.toLowerCase()),
      )
      return filteredTemplates.map((template) => ({
        label:
          template.display_name !== "" ? template.display_name : template.name,
        value: template.name,
        icon: template.icon,
      }))
    },
    onChange,
  })
}

type TemplatesAutocomplete = ReturnType<typeof useTemplatesAutocomplete>

export const useStatusAutocomplete = (
  initialOptionValue: string | undefined,
  onChange: (option: StatusOption | undefined) => void,
) => {
  const statusOptions = WorkspaceStatuses.map((status) => {
    const display = getDisplayWorkspaceStatus(status)
    return {
      label: display.text,
      value: status,
      color: display.type ?? "warning",
    } as StatusOption
  })
  return useAutocomplete({
    id: "status",
    getInitialOption: async () =>
      statusOptions.find((option) => option.value === initialOptionValue) ??
      null,
    getOptions: async () => statusOptions,
    onChange,
  })
}

type StatusAutocomplete = ReturnType<typeof useStatusAutocomplete>

/** Components */

const FilterSkeleton = (props: SkeletonProps) => {
  return (
    <Skeleton
      variant="rectangular"
      height={36}
      {...props}
      sx={{
        bgcolor: (theme) => theme.palette.background.paperLight,
        borderRadius: "6px",
      }}
    />
  )
}

export const Filter = ({
  filter,
  autocomplete,
}: {
  filter: UseFilterResult
  autocomplete: {
    users: UsersAutocomplete
    templates: TemplatesAutocomplete
    status: StatusAutocomplete
  }
}) => {
  const hasFilterQuery = filter.query !== ""
  const isIinitializingFilters =
    autocomplete.status.isInitializing ||
    autocomplete.templates.isInitializing ||
    autocomplete.users.isInitializing

  if (isIinitializingFilters) {
    return (
      <Box display="flex" sx={{ gap: 1, mb: 2 }}>
        <FilterSkeleton width="100%" />
        <FilterSkeleton width="200px" />
        <FilterSkeleton width="200px" />
        <FilterSkeleton width="200px" />
      </Box>
    )
  }

  return (
    <Box display="flex" sx={{ gap: 1, mb: 2 }}>
      <TextField
        sx={{ width: "100%" }}
        color="success"
        size="small"
        InputProps={{
          placeholder: "Search...",
          value: filter.query,
          onChange: (e) => filter.update(e.target.value),
          sx: {
            borderRadius: "6px",
            "& input::placeholder": {
              color: (theme) => theme.palette.text.secondary,
            },
          },
          startAdornment: (
            <InputAdornment position="start">
              <SearchOutlined
                sx={{
                  fontSize: 14,
                  color: (theme) => theme.palette.text.secondary,
                }}
              />
            </InputAdornment>
          ),
          endAdornment: hasFilterQuery && (
            <InputAdornment position="end">
              <Tooltip title="Clear filter">
                <IconButton
                  size="small"
                  onClick={() => {
                    filter.update("")
                    autocomplete.users.clearSelection()
                    autocomplete.templates.clearSelection()
                    autocomplete.status.clearSelection()
                  }}
                >
                  <CloseOutlined sx={{ fontSize: 14 }} />
                </IconButton>
              </Tooltip>
            </InputAdornment>
          ),
        }}
      />
      <OwnerFilter autocomplete={autocomplete.users} />
      <TemplatesFilter autocomplete={autocomplete.templates} />
      <StatusFilter autocomplete={autocomplete.status} />
    </Box>
  )
}

const OwnerFilter = ({ autocomplete }: { autocomplete: UsersAutocomplete }) => {
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)

  const handleClose = () => {
    setIsMenuOpen(false)
  }

  return (
    <div>
      <MenuButton
        ref={buttonRef}
        onClick={() => setIsMenuOpen(true)}
        sx={{ width: 200 }}
      >
        {autocomplete.selectedOption ? (
          <UserOptionItem option={autocomplete.selectedOption} />
        ) : (
          "All users"
        )}
      </MenuButton>
      <SearchMenu
        id="user-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        options={autocomplete.searchOptions}
        query={autocomplete.query}
        onQueryChange={autocomplete.setQuery}
        renderOption={(option) => (
          <MenuItem
            key={option.label}
            selected={option.value === autocomplete.selectedOption?.value}
            onClick={() => {
              autocomplete.selectOption(option)
              handleClose()
            }}
          >
            <UserOptionItem option={option} />
          </MenuItem>
        )}
      />
    </div>
  )
}

const UserOptionItem = ({ option }: { option: OwnerOption }) => {
  return (
    <Box
      display="flex"
      alignItems="center"
      gap={2}
      fontSize={14}
      overflow="hidden"
    >
      <UserAvatar
        username={option.label}
        avatarURL={option.avatarUrl}
        sx={{ width: 16, height: 16, fontSize: 8 }}
      />
      <Box component="span" overflow="hidden" textOverflow="ellipsis">
        {option.label}
      </Box>
    </Box>
  )
}

const TemplatesFilter = ({
  autocomplete,
}: {
  autocomplete: TemplatesAutocomplete
}) => {
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)

  const handleClose = () => {
    setIsMenuOpen(false)
  }

  return (
    <div>
      <MenuButton
        ref={buttonRef}
        onClick={() => setIsMenuOpen(true)}
        sx={{ width: 200 }}
      >
        {autocomplete.selectedOption ? (
          <TemplateOptionItem option={autocomplete.selectedOption} />
        ) : (
          "All templates"
        )}
      </MenuButton>
      <SearchMenu
        id="template-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        options={autocomplete.searchOptions}
        query={autocomplete.query}
        onQueryChange={autocomplete.setQuery}
        renderOption={(option) => (
          <MenuItem
            key={option.label}
            selected={option.value === autocomplete.selectedOption?.value}
            onClick={() => {
              autocomplete.selectOption(option)
              handleClose()
            }}
          >
            <TemplateOptionItem option={option} />
          </MenuItem>
        )}
      />
    </div>
  )
}

const TemplateOptionItem = ({ option }: { option: TemplateOption }) => {
  return (
    <Box
      display="flex"
      alignItems="center"
      gap={2}
      fontSize={14}
      overflow="hidden"
    >
      <TemplateAvatar
        templateName={option.label}
        icon={option.icon}
        sx={{ width: 14, height: 14, fontSize: 8 }}
      />
      <Box component="span" overflow="hidden" textOverflow="ellipsis">
        {option.label}
      </Box>
    </Box>
  )
}

const TemplateAvatar: FC<
  AvatarProps & { templateName: string; icon?: string }
> = ({ templateName, icon, ...avatarProps }) => {
  return icon ? (
    <Avatar src={icon} variant="square" fitImage {...avatarProps} />
  ) : (
    <Avatar {...avatarProps}>{templateName}</Avatar>
  )
}

const StatusFilter = ({
  autocomplete,
}: {
  autocomplete: StatusAutocomplete
}) => {
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)

  const handleClose = () => {
    setIsMenuOpen(false)
  }

  return (
    <div>
      <MenuButton
        ref={buttonRef}
        onClick={() => setIsMenuOpen(true)}
        sx={{ width: 200 }}
      >
        {autocomplete.selectedOption ? (
          <StatusOptionItem option={autocomplete.selectedOption} />
        ) : (
          "All statuses"
        )}
      </MenuButton>
      <Menu
        id="status-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        sx={{ "& .MuiPaper-root": { minWidth: 200 } }}
        // Disabled this so when we clear the filter and do some sorting in the
        // search items it does not look strange. Github removes exit transitions
        // on their filters as well.
        transitionDuration={{
          enter: 250,
          exit: 0,
        }}
      >
        {autocomplete.searchOptions?.map((option) => (
          <MenuItem
            key={option.label}
            selected={option.value === autocomplete.selectedOption?.value}
            onClick={() => {
              autocomplete.selectOption(option)
              handleClose()
            }}
          >
            <StatusOptionItem option={option} />
          </MenuItem>
        ))}
      </Menu>
    </div>
  )
}

const StatusOptionItem = ({ option }: { option: StatusOption }) => {
  return (
    <Box display="flex" alignItems="center" gap={2} fontSize={14}>
      <StatusIndicator option={option} />
      <span>{option.label}</span>
    </Box>
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

const MenuButton = forwardRef<HTMLButtonElement, ButtonProps>((props, ref) => {
  return (
    <Button
      ref={ref}
      endIcon={<KeyboardArrowDown />}
      {...props}
      sx={{
        borderRadius: "6px",
        justifyContent: "space-between",
        lineHeight: 1,
        ...props.sx,
      }}
    />
  )
})

function SearchMenu<TOption extends { label: string; value: string }>({
  options,
  renderOption,
  query,
  onQueryChange,
  ...menuProps
}: Pick<MenuProps, "anchorEl" | "open" | "onClose" | "id"> & {
  options?: TOption[]
  renderOption: (option: TOption) => ReactNode
  query: string
  onQueryChange: (query: string) => void
}) {
  const menuListRef = useRef<HTMLUListElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)

  return (
    <Menu
      {...menuProps}
      onClose={(event, reason) => {
        menuProps.onClose && menuProps.onClose(event, reason)
        onQueryChange("")
      }}
      sx={{
        "& .MuiPaper-root": {
          width: 320,
          paddingY: 0,
        },
      }}
      // Disabled this so when we clear the filter and do some sorting in the
      // search items it does not look strange. Github removes exit transitions
      // on their filters as well.
      transitionDuration={{
        enter: 250,
        exit: 0,
      }}
    >
      <Box
        component="li"
        sx={{
          display: "flex",
          alignItems: "center",
          paddingLeft: 2,
          height: 40,
          borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
        }}
        onKeyDown={(e) => {
          e.stopPropagation()
          if (e.key === "ArrowDown" && menuListRef.current) {
            const firstItem = menuListRef.current.firstChild as HTMLElement
            firstItem.focus()
          }
        }}
      >
        <SearchOutlined
          sx={{
            fontSize: 14,
            color: (theme) => theme.palette.text.secondary,
          }}
        />
        <Box
          tabIndex={-1}
          component="input"
          type="text"
          placeholder="Search..."
          autoFocus
          value={query}
          ref={searchInputRef}
          onChange={(e) => {
            onQueryChange(e.target.value)
          }}
          sx={{
            height: "100%",
            border: 0,
            background: "none",
            width: "100%",
            marginLeft: 2,
            outline: 0,
            "&::placeholder": {
              color: (theme) => theme.palette.text.secondary,
            },
          }}
        />
      </Box>

      <Box component="li" sx={{ maxHeight: 480, overflowY: "auto" }}>
        <MenuList
          ref={menuListRef}
          onKeyDown={(e) => {
            if (e.shiftKey && e.code === "Tab") {
              e.preventDefault()
              e.stopPropagation()
              searchInputRef.current?.focus()
            }
          }}
        >
          {options ? (
            options.length > 0 ? (
              options.map(renderOption)
            ) : (
              <Box
                sx={{
                  fontSize: 13,
                  color: (theme) => theme.palette.text.secondary,
                  textAlign: "center",
                  py: 1,
                }}
              >
                No results
              </Box>
            )
          ) : (
            <Loader size={14} />
          )}
        </MenuList>
      </Box>
    </Menu>
  )
}
