import Button from "@material-ui/core/Button"
import Fade from "@material-ui/core/Fade"
import InputAdornment from "@material-ui/core/InputAdornment"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import OutlinedInput from "@material-ui/core/OutlinedInput"
import { makeStyles } from "@material-ui/core/styles"
import { Theme } from "@material-ui/core/styles/createTheme"
import SearchIcon from "@material-ui/icons/Search"
import debounce from "just-debounce-it"
import { useCallback, useRef, useState } from "react"
import { getValidationErrorMessage } from "../../api/errors"
import { CloseDropdown, OpenDropdown } from "../DropdownArrows/DropdownArrows"
import { Stack } from "../Stack/Stack"

export const Language = {
  filterName: "Filters",
}

export interface SearchBarWithFilterProps {
  filter?: string
  onFilter: (query: string) => void
  presetFilters?: PresetFilter[]
  error?: unknown
  docs?: string
}

export interface PresetFilter {
  name: string
  query: string
}

export const SearchBarWithFilter: React.FC<
  React.PropsWithChildren<SearchBarWithFilterProps>
> = ({ filter, onFilter, presetFilters, error, docs }) => {
  const styles = useStyles({ error: Boolean(error) })
  const searchInputRef = useRef<HTMLInputElement>(null)

  // debounce query string entry by user
  // we want the dependency array empty here
  // as we don't need to redefine the function
  // eslint-disable-next-line react-hooks/exhaustive-deps -- see above
  const debouncedOnFilter = useCallback(
    debounce((debouncedQueryString: string) => {
      onFilter(debouncedQueryString)
    }, 300),
    [],
  )

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const setPresetFilter = (query: string) => () => {
    if (!searchInputRef.current) {
      throw new Error("Search input not found.")
    }

    onFilter(query)
    // Update this to the input directly instead of create a new state and
    // re-render the component since the onFilter is already calling the
    // filtering process
    searchInputRef.current.value = query
    handleClose()
  }

  const errorMessage = getValidationErrorMessage(error)

  return (
    <Stack spacing={1} className={styles.root}>
      <Stack direction="row" spacing={0}>
        {presetFilters && presetFilters.length > 0 && (
          <Button
            variant="outlined"
            aria-controls="filter-menu"
            aria-haspopup="true"
            onClick={handleClick}
            className={styles.buttonRoot}
          >
            {Language.filterName}{" "}
            {anchorEl ? <CloseDropdown /> : <OpenDropdown />}
          </Button>
        )}

        <div role="form" className={styles.filterForm}>
          <OutlinedInput
            id="query"
            name="query"
            defaultValue={filter}
            error={Boolean(error)}
            className={styles.inputStyles}
            onChange={(event) => {
              debouncedOnFilter(event.currentTarget.value)
            }}
            inputRef={searchInputRef}
            inputProps={{
              "aria-label": "Filter",
            }}
            startAdornment={
              <InputAdornment position="start" className={styles.searchIcon}>
                <SearchIcon fontSize="small" />
              </InputAdornment>
            }
          />
        </div>

        {presetFilters && presetFilters.length > 0 ? (
          <Menu
            id="filter-menu"
            anchorEl={anchorEl}
            keepMounted
            open={Boolean(anchorEl)}
            onClose={handleClose}
            TransitionComponent={Fade}
            anchorOrigin={{
              vertical: "bottom",
              horizontal: "left",
            }}
            transformOrigin={{
              vertical: "top",
              horizontal: "left",
            }}
          >
            {presetFilters.map((presetFilter) => (
              <MenuItem
                key={presetFilter.name}
                onClick={setPresetFilter(presetFilter.query)}
              >
                {presetFilter.name}
              </MenuItem>
            ))}
            {docs && (
              <MenuItem component="a" href={docs} target="_blank">
                View advanced filtering
              </MenuItem>
            )}
          </Menu>
        ) : null}
      </Stack>
      {errorMessage && (
        <Stack className={styles.errorRoot}>{errorMessage}</Stack>
      )}
    </Stack>
  )
}

interface StyleProps {
  error?: boolean
}

const useStyles = makeStyles<Theme, StyleProps>((theme) => ({
  root: {
    marginBottom: theme.spacing(2),

    "&:has(button) .MuiInputBase-root": {
      borderTopLeftRadius: 0,
      borderBottomLeftRadius: 0,
    },
  },
  // necessary to expand the textField
  // the length of the page (within the bordered filterContainer)
  filterForm: {
    width: "100%",
  },
  buttonRoot: {
    border: `1px solid ${theme.palette.divider}`,
    borderRight: "0px",
    borderRadius: `${theme.shape.borderRadius}px 0px 0px ${theme.shape.borderRadius}px`,
    flexShrink: 0,
  },
  errorRoot: {
    color: theme.palette.error.main,
    whiteSpace: "pre-wrap",
  },
  inputStyles: {
    height: "100%",
    width: "100%",
    color: theme.palette.primary.contrastText,
    backgroundColor: theme.palette.background.paper,

    "& fieldset": {
      borderColor: theme.palette.divider,
      "&MuiOutlinedInput-root:hover, &MuiOutlinedInput-notchedOutline": {
        borderColor: (props) => props.error && theme.palette.error.contrastText,
      },
    },

    "& .MuiInputBase-input": {
      paddingTop: "inherit",
      paddingBottom: "inherit",
      // The same as the button
      minHeight: 40,
    },
  },
  searchIcon: {
    color: theme.palette.text.secondary,
  },
}))
