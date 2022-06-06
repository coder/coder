import Button from "@material-ui/core/Button"
import Fade from "@material-ui/core/Fade"
import InputAdornment from "@material-ui/core/InputAdornment"
import Link from "@material-ui/core/Link"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import SearchIcon from "@material-ui/icons/Search"
import { useMachine } from "@xstate/react"
import { FormikErrors, useFormik } from "formik"
import { FC, useState } from "react"
import { Helmet } from "react-helmet"
import { Link as RouterLink } from "react-router-dom"
import { CloseDropdown, OpenDropdown } from "../../components/DropdownArrows/DropdownArrows"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { pageTitle } from "../../util/page"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

interface FilterFormValues {
  query: string
}

const Language = {
  filterName: "Filters",
  createWorkspaceButton: "Create workspace",
  yourWorkspacesButton: "Your workspaces",
  allWorkspacesButton: "All workspaces",
}

export type FilterFormErrors = FormikErrors<FilterFormValues>

const WorkspacesPage: FC = () => {
  const styles = useStyles()
  const [workspacesState, send] = useMachine(workspacesMachine)

  const form = useFormik<FilterFormValues>({
    initialValues: {
      query: workspacesState.context.filter || "",
    },
    onSubmit: (values) => {
      send({
        type: "SET_FILTER",
        query: values.query,
      })
    },
  })

  const getFieldHelpers = getFormHelpers<FilterFormValues>(form)

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)

  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget)
  }

  const handleClose = () => {
    setAnchorEl(null)
  }

  const setYourWorkspaces = () => {
    void form.setFieldValue("query", "owner:me")
    void form.submitForm()
    handleClose()
  }

  const setAllWorkspaces = () => {
    void form.setFieldValue("query", "")
    void form.submitForm()
    handleClose()
  }

  return (
    <Margins>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>
      <Stack direction="row" className={styles.workspacesHeaderContainer}>
        <Stack direction="column" className={styles.filterColumn}>
          <Stack direction="row" spacing={0} className={styles.filterContainer}>
            <Button
              aria-controls="filter-menu"
              aria-haspopup="true"
              onClick={handleClick}
              className={styles.buttonRoot}
            >
              {Language.filterName} {anchorEl ? <CloseDropdown /> : <OpenDropdown />}
            </Button>

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
              <MenuItem onClick={setYourWorkspaces}>{Language.yourWorkspacesButton}</MenuItem>
              <MenuItem onClick={setAllWorkspaces}>{Language.allWorkspacesButton}</MenuItem>
            </Menu>
          </Stack>
        </Stack>

        <Link underline="none" component={RouterLink} to="/workspaces/new">
          <Button startIcon={<AddCircleOutline />} style={{ height: "44px" }}>
            {Language.createWorkspaceButton}
          </Button>
        </Link>
      </Stack>
      <WorkspacesPageView loading={workspacesState.hasTag("loading")} workspaces={workspacesState.context.workspaces} />
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  workspacesHeaderContainer: {
    marginTop: theme.spacing(3),
    marginBottom: theme.spacing(3),
    justifyContent: "space-between",
  },
  filterColumn: {
    width: "60%",
    cursor: "text",
  },
  filterContainer: {
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: "6px",
  },
  filterForm: {
    width: "100%",
  },
  buttonRoot: {
    border: "none",
    borderRight: `1px solid ${theme.palette.divider}`,
    borderRadius: "6px 0px 0px 6px",
  },
  textFieldRoot: {
    margin: "0px",
    "& fieldset": {
      border: "none",
    },
  },
}))

export default WorkspacesPage
