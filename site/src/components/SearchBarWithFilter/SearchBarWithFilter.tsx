import Button from "@material-ui/core/Button"
import Fade from "@material-ui/core/Fade"
import InputAdornment from "@material-ui/core/InputAdornment"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import OutlinedInput from "@material-ui/core/OutlinedInput"
import { makeStyles } from "@material-ui/core/styles"
import { Theme } from "@material-ui/core/styles/createMuiTheme"
import SearchIcon from "@material-ui/icons/Search"
import { FormikErrors, useFormik } from "formik"
import debounce from "just-debounce-it"
import { useCallback, useEffect, useState } from "react"
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
}

export interface PresetFilter {
  name: string
  query: string
}

interface FilterFormValues {
  query: string
}

export type FilterFormErrors = FormikErrors<FilterFormValues>

export const SearchBarWithFilter: React.FC<React.PropsWithChildren<SearchBarWithFilterProps>> = ({
  filter,
  onFilter,
  presetFilters,
  error,
}) => {
  const styles = useStyles({ error: !!error })

  const form = useFormik<FilterFormValues>({
    enableReinitialize: true,
    initialValues: {
      query: filter ?? "",
    },
    onSubmit: ({ query }) => {
      onFilter(query)
    },
  })

  // debounce query string entry by user
  // we want the dependency array empty here
  // as we don't need to redefine the function
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const debouncedOnFilter = useCallback(
    debounce((debouncedQueryString: string) => {
      onFilter(debouncedQueryString)
    }, 300),
    [],
  )

  // update the query params while typing
  useEffect(() => {
    debouncedOnFilter(form.values.query)
    return () => debouncedOnFilter.cancel()
  }, [debouncedOnFilter, form.values.query])

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const setPresetFilter = (query: string) => () => {
    void form.setFieldValue("query", query)
    void form.submitForm()
    handleClose()
  }

  const errorMessage = getValidationErrorMessage(error)

  return (
    <Stack spacing={1} className={styles.root}>
      <Stack direction="row" spacing={0}>
        {presetFilters && presetFilters.length > 0 && (
          <Button
            aria-controls="filter-menu"
            aria-haspopup="true"
            onClick={handleClick}
            className={styles.buttonRoot}
          >
            {Language.filterName} {anchorEl ? <CloseDropdown /> : <OpenDropdown />}
          </Button>
        )}

        <form onSubmit={form.handleSubmit} className={styles.filterForm}>
          <OutlinedInput
            id="query"
            name="query"
            value={form.values.query}
            error={!!error}
            className={styles.inputStyles}
            onChange={form.handleChange}
            startAdornment={
              <InputAdornment position="start" className={styles.searchIcon}>
                <SearchIcon fontSize="small" />
              </InputAdornment>
            }
          />
        </form>

        {presetFilters && presetFilters.length && (
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
              <MenuItem key={presetFilter.name} onClick={setPresetFilter(presetFilter.query)}>
                {presetFilter.name}
              </MenuItem>
            ))}
          </Menu>
        )}
      </Stack>
      {errorMessage && <Stack className={styles.errorRoot}>{errorMessage}</Stack>}
    </Stack>
  )
}

interface StyleProps {
  error?: boolean
}

const useStyles = makeStyles<Theme, StyleProps>((theme) => ({
  root: {
    marginBottom: theme.spacing(2),
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
  },
  inputStyles: {
    height: "100%",
    width: "100%",
    borderRadius: `0px ${theme.shape.borderRadius}px ${theme.shape.borderRadius}px 0px`,
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
      minHeight: 42,
    },
  },
  searchIcon: {
    color: theme.palette.text.secondary,
  },
}))
