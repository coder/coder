import Button from "@material-ui/core/Button"
import Fade from "@material-ui/core/Fade"
import InputAdornment from "@material-ui/core/InputAdornment"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import SearchIcon from "@material-ui/icons/Search"
import { FormikErrors, useFormik } from "formik"
import debounce from "just-debounce-it"
import { useCallback, useEffect, useState } from "react"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { CloseDropdown, OpenDropdown } from "../DropdownArrows/DropdownArrows"
import { Stack } from "../Stack/Stack"

export const Language = {
  filterName: "Filters",
}

export interface SearchBarWithFilterProps {
  filter?: string
  onFilter: (query: string) => void
  presetFilters?: PresetFilter[]
}

export interface PresetFilter {
  name: string
  query: string
}

interface FilterFormValues {
  query: string
}

export type FilterFormErrors = FormikErrors<FilterFormValues>

export const SearchBarWithFilter: React.FC<SearchBarWithFilterProps> = ({
  filter,
  onFilter,
  presetFilters,
}) => {
  const styles = useStyles()

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

  const getFieldHelpers = getFormHelpers<FilterFormValues>(form)

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

  return (
    <Stack direction="row" spacing={0} className={styles.filterContainer}>
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
        <TextField
          {...getFieldHelpers("query")}
          className={styles.textFieldRoot}
          onChange={onChangeTrimmed(form)}
          fullWidth
          variant="outlined"
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon fontSize="small" />
              </InputAdornment>
            ),
          }}
        />
      </form>

      {presetFilters && presetFilters.length > 0 && (
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
  )
}

const useStyles = makeStyles((theme) => ({
  filterContainer: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: theme.shape.borderRadius,
    marginBottom: theme.spacing(2),
  },
  filterForm: {
    width: "100%",
  },
  buttonRoot: {
    border: "none",
    borderRight: `1px solid ${theme.palette.divider}`,
    borderRadius: `${theme.shape.borderRadius}px 0px 0px ${theme.shape.borderRadius}px`,
  },
  textFieldRoot: {
    margin: "0px",
    "& fieldset": {
      border: "none",
    },
  },
}))
