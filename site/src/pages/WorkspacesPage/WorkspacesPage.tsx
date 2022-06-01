import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import { useMachine } from "@xstate/react"
import { FormikContextType, FormikErrors, useFormik } from "formik"
import { FC, useState } from "react"
import { Link as RouterLink } from "react-router-dom"
import { Margins } from "../../components/Margins/Margins"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { Language, WorkspacesPageView } from "./WorkspacesPageView"

interface FilterFormValues {
  query: string
}

export type FilterFormErrors = FormikErrors<FilterFormValues>

const WorkspacesPage: FC = () => {
  const styles = useStyles()
  const [workspacesState, send] = useMachine(workspacesMachine)

  const form: FormikContextType<FilterFormValues> = useFormik<FilterFormValues>({
    initialValues: { query: workspacesState.context.filter || "" },
    onSubmit: (data) => {
      send({
        type: "SET_FILTER",
        query: data.query,
      })
    },
  })

  const getFieldHelpers = getFormHelpers<FilterFormValues>(form, {})

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const handleClick = (event: React.MouseEvent<HTMLButtonElement>) => {
    setAnchorEl(event.currentTarget)
  }
  const handleClose = () => {
    setAnchorEl(null)
  }
  const setYourWorkspaces = () => {
    form.setFieldValue("query", "owner:me")
    void form.submitForm()
    handleClose()
  }
  const setAllWorkspaces = () => {
    form.setFieldValue("query", "")
    void form.submitForm()
    handleClose()
  }

  return (
    <>
      <Margins>
        <div className={styles.actions}>
          <Button aria-controls="simple-menu" aria-haspopup="true" onClick={handleClick}>
            Filter
          </Button>
          <Menu id="simple-menu" anchorEl={anchorEl} keepMounted open={Boolean(anchorEl)} onClose={handleClose}>
            <MenuItem onClick={setYourWorkspaces}>Your workspaces</MenuItem>
            <MenuItem onClick={setAllWorkspaces}>All workspaces</MenuItem>
          </Menu>
          <form onSubmit={form.handleSubmit}>
            <TextField {...getFieldHelpers("query")} onChange={onChangeTrimmed(form)} fullWidth variant="outlined" />
          </form>
          <Link underline="none" component={RouterLink} to="/workspaces/new">
            <Button startIcon={<AddCircleOutline />}>{Language.createButton}</Button>
          </Link>
        </div>
        <WorkspacesPageView
          loading={workspacesState.hasTag("loading")}
          workspaces={workspacesState.context.workspaces}
          error={workspacesState.context.getWorkspacesError}
        />
      </Margins>
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  actions: {
    marginTop: theme.spacing(3),
    marginBottom: theme.spacing(3),
    display: "flex",
    justifyContent: "space-between",
    height: theme.spacing(6),
  },
}))

export default WorkspacesPage
