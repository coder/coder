import { WorkspaceStatuses } from "api/typesGenerated"
import { FC, ReactNode, forwardRef, useRef, useState } from "react"
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
  const [selectedOption, setSelectedOption] = useState<BaseOption>()
  const initialOptionQuery = useQuery({
    queryKey: [id, "autocomplete", "initial"],
    queryFn: () => getInitialOption(),
    onSuccess: setSelectedOption,
  })
  const searchOptionsQuery = useQuery({
    queryKey: [id, "autoComplete", "search"],
    queryFn: () => getOptions(query),
  })
  const selectOption = (option: TOption) => {
    let newSelectedOptionValue: TOption | undefined = option

    if (option.label === selectedOption?.value) {
      newSelectedOptionValue = undefined
    }

    if (onChange) {
      onChange(newSelectedOptionValue)
    }
    setSelectedOption(newSelectedOptionValue)
  }

  return {
    query,
    setQuery,
    selectedOption,
    selectOption,
    isInitializing: initialOptionQuery.isInitialLoading,
    initialOption: initialOptionQuery.data,
    isSearching: searchOptionsQuery.isFetching,
    searchOptions: searchOptionsQuery.data,
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
                <IconButton size="small" onClick={() => filter.update("")}>
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
      <MenuButton ref={buttonRef} onClick={() => setIsMenuOpen(true)}>
        User
      </MenuButton>
      <SearchMenu
        id="user-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        options={autocomplete.searchOptions}
        renderOption={(option) => (
          <MenuItem
            key={option.label}
            selected={option.value === autocomplete.selectedOption?.value}
            onClick={() => {
              autocomplete.selectOption(option)
              handleClose()
            }}
          >
            <Box display="flex" alignItems="center" gap={2} fontSize={14}>
              <UserAvatar
                username={option.label}
                avatarURL={option.avatarUrl}
                sx={{ width: 16, height: 16, fontSize: 8 }}
              />
              <span>{option.label}</span>
            </Box>
          </MenuItem>
        )}
      />
    </div>
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
      <MenuButton ref={buttonRef} onClick={() => setIsMenuOpen(true)}>
        Template
      </MenuButton>
      <SearchMenu
        id="template-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        options={autocomplete.searchOptions}
        renderOption={(option) => (
          <MenuItem
            key={option.label}
            selected={option.value === autocomplete.selectedOption?.value}
            onClick={() => {
              autocomplete.selectOption(option)
              handleClose()
            }}
          >
            <Box display="flex" alignItems="center" gap={2} fontSize={14}>
              <TemplateAvatar
                templateName={option.label}
                icon={option.icon}
                sx={{ width: 14, height: 14, fontSize: 8 }}
              />
              <span>{option.label}</span>
            </Box>
          </MenuItem>
        )}
      />
    </div>
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
      <MenuButton ref={buttonRef} onClick={() => setIsMenuOpen(true)}>
        Status
      </MenuButton>
      <Menu
        id="status-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
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
            <Box display="flex" alignItems="center" gap={2} fontSize={14}>
              <StatusIndicator option={option} />
              <span>{option.label}</span>
            </Box>
          </MenuItem>
        ))}
      </Menu>
    </div>
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
        lineHeight: 0,
        ...props.sx,
      }}
    />
  )
})

function SearchMenu<TOption extends { label: string; value: string }>({
  options,
  renderOption,
  ...menuProps
}: Pick<MenuProps, "anchorEl" | "open" | "onClose" | "id"> & {
  options?: TOption[]
  renderOption: (option: TOption) => ReactNode
}) {
  const menuListRef = useRef<HTMLUListElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const [searchInputValue, setSearchInputValue] = useState("")
  const visibleOptions = options
    ? options.filter(
        (option) =>
          option.label.toLowerCase().includes(searchInputValue.toLowerCase()) ||
          option.value.toLowerCase().includes(searchInputValue.toLowerCase()),
      )
    : undefined

  return (
    <Menu
      {...menuProps}
      onClose={(event, reason) => {
        menuProps.onClose && menuProps.onClose(event, reason)
        // 250ms is the transition time to close menu
        setTimeout(() => setSearchInputValue(""), 250)
      }}
      sx={{
        "& .MuiPaper-root": {
          width: 320,
          paddingY: 0,
        },
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
          value={searchInputValue}
          ref={searchInputRef}
          onChange={(e) => {
            setSearchInputValue(e.target.value)
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
          {visibleOptions ? (
            visibleOptions.length > 0 ? (
              visibleOptions.map(renderOption)
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
