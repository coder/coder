import { FC, ReactNode, forwardRef, useEffect, useRef, useState } from "react"
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
import { Loader } from "components/Loader/Loader"
import MenuList from "@mui/material/MenuList"
import { useSearchParams } from "react-router-dom"
import Skeleton, { SkeletonProps } from "@mui/material/Skeleton"
import CheckOutlined from "@mui/icons-material/CheckOutlined"
import {
  getValidationErrorMessage,
  hasError,
  isApiValidationError,
} from "api/errors"
import {
  UsersAutocomplete,
  TemplatesAutocomplete,
  StatusAutocomplete,
} from "./autocompletes"
import {
  OwnerOption,
  TemplateOption,
  StatusOption,
  BaseOption,
} from "./options"
import debounce from "just-debounce-it"

export type FilterValues = {
  owner?: string // User["username"]
  status?: string // WorkspaceStatus
  template?: string // Template["name"]
}

export const useFilter = ({
  onUpdate,
  searchParamsResult,
}: {
  searchParamsResult: ReturnType<typeof useSearchParams>
  onUpdate?: () => void
}) => {
  const [searchParams, setSearchParams] = searchParamsResult
  const query = searchParams.get("filter") ?? ""
  const values = parseFilterQuery(query)

  const update = (values: string | FilterValues) => {
    if (typeof values === "string") {
      searchParams.set("filter", values)
    } else {
      searchParams.set("filter", stringifyFilter(values))
    }
    setSearchParams(searchParams)
    if (onUpdate) {
      onUpdate()
    }
  }

  const debounceUpdate = debounce(
    (values: string | FilterValues) => update(values),
    500,
  )

  return {
    query,
    update,
    debounceUpdate,
    values,
  }
}

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

const FilterSkeleton = (props: SkeletonProps) => {
  return (
    <Skeleton
      variant="rectangular"
      height={36}
      {...props}
      sx={{
        bgcolor: (theme) => theme.palette.background.paperLight,
        borderRadius: "6px",
        ...props.sx,
      }}
    />
  )
}

export const Filter = ({
  filter,
  autocomplete,
  error,
}: {
  filter: ReturnType<typeof useFilter>
  error?: unknown
  autocomplete: {
    users?: UsersAutocomplete
    templates: TemplatesAutocomplete
    status: StatusAutocomplete
  }
}) => {
  const shouldDisplayError = hasError(error) && isApiValidationError(error)
  const hasFilterQuery = filter.query !== ""
  const isIinitializingFilters =
    autocomplete.status.isInitializing ||
    autocomplete.templates.isInitializing ||
    (autocomplete.users && autocomplete.users.isInitializing)
  const [searchQuery, setSearchQuery] = useState(filter.query)

  useEffect(() => {
    setSearchQuery(filter.query)
  }, [filter.query])

  if (isIinitializingFilters) {
    return (
      <Box display="flex" sx={{ gap: 1, mb: 2 }}>
        <FilterSkeleton width="100%" />
        {autocomplete.users && (
          <FilterSkeleton width="200px" sx={{ flexShrink: 0 }} />
        )}
        <FilterSkeleton width="200px" sx={{ flexShrink: 0 }} />
        <FilterSkeleton width="200px" sx={{ flexShrink: 0 }} />
      </Box>
    )
  }

  return (
    <Box display="flex" sx={{ gap: 1, mb: 2 }}>
      <TextField
        fullWidth
        error={shouldDisplayError}
        helperText={
          shouldDisplayError ? getValidationErrorMessage(error) : undefined
        }
        size="small"
        InputProps={{
          name: "query",
          placeholder: "Search...",
          value: searchQuery,
          onChange: (e) => {
            setSearchQuery(e.target.value)
            filter.debounceUpdate(e.target.value)
          },
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
                  }}
                >
                  <CloseOutlined sx={{ fontSize: 14 }} />
                </IconButton>
              </Tooltip>
            </InputAdornment>
          ),
        }}
      />

      {autocomplete.users && <OwnerFilter autocomplete={autocomplete.users} />}
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
            <UserOptionItem
              option={option}
              isSelected={option.value === autocomplete.selectedOption?.value}
            />
          </MenuItem>
        )}
      />
    </div>
  )
}

const UserOptionItem = ({
  option,
  isSelected,
}: {
  option: OwnerOption
  isSelected?: boolean
}) => {
  return (
    <OptionItem
      option={option}
      isSelected={isSelected}
      left={
        <UserAvatar
          username={option.label}
          avatarURL={option.avatarUrl}
          sx={{ width: 16, height: 16, fontSize: 8 }}
        />
      }
    />
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
            <TemplateOptionItem
              option={option}
              isSelected={option.value === autocomplete.selectedOption?.value}
            />
          </MenuItem>
        )}
      />
    </div>
  )
}

const TemplateOptionItem = ({
  option,
  isSelected,
}: {
  option: TemplateOption
  isSelected?: boolean
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
            <StatusOptionItem
              option={option}
              isSelected={option.value === autocomplete.selectedOption?.value}
            />
          </MenuItem>
        ))}
      </Menu>
    </div>
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

type OptionItemProps = {
  option: BaseOption
  left?: ReactNode
  isSelected?: boolean
}

const OptionItem = ({ option, left, isSelected }: OptionItemProps) => {
  return (
    <Box
      display="flex"
      alignItems="center"
      gap={2}
      fontSize={14}
      overflow="hidden"
      width="100%"
    >
      {left}
      <Box component="span" overflow="hidden" textOverflow="ellipsis">
        {option.label}
      </Box>
      {isSelected && (
        <CheckOutlined sx={{ width: 16, height: 16, marginLeft: "auto" }} />
      )}
    </Box>
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
        lineHeight: "120%",
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
