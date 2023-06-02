import { FC, ReactNode, forwardRef, useEffect, useRef, useState } from "react"
import Box from "@mui/material/Box"
import TextField from "@mui/material/TextField"
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown"
import Button, { ButtonProps } from "@mui/material/Button"
import Menu from "@mui/material/Menu"
import MenuItem from "@mui/material/MenuItem"
import SearchOutlined from "@mui/icons-material/SearchOutlined"
import InputAdornment from "@mui/material/InputAdornment"
import { Palette, PaletteColor } from "@mui/material/styles"
import IconButton from "@mui/material/IconButton"
import Tooltip from "@mui/material/Tooltip"
import CloseOutlined from "@mui/icons-material/CloseOutlined"
import { useSearchParams } from "react-router-dom"
import Skeleton, { SkeletonProps } from "@mui/material/Skeleton"
import CheckOutlined from "@mui/icons-material/CheckOutlined"
import {
  getValidationErrorMessage,
  hasError,
  isApiValidationError,
} from "api/errors"
import { StatusAutocomplete } from "./autocompletes"
import { StatusOption, BaseOption } from "./options"
import debounce from "just-debounce-it"

export type FilterValues = {
  status?: string
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
    status: StatusAutocomplete
  }
}) => {
  const shouldDisplayError = hasError(error) && isApiValidationError(error)
  const hasFilterQuery = filter.query !== ""
  const isIinitializingFilters = autocomplete.status.isInitializing
  const [searchQuery, setSearchQuery] = useState(filter.query)

  useEffect(() => {
    setSearchQuery(filter.query)
  }, [filter.query])

  if (isIinitializingFilters) {
    return (
      <Box display="flex" sx={{ gap: 1, mb: 2 }}>
        <FilterSkeleton width="100%" />
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

      <StatusFilter autocomplete={autocomplete.status} />
    </Box>
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
