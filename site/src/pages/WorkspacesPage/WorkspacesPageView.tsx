import Button from "@material-ui/core/Button"
import Fade from "@material-ui/core/Fade"
import InputAdornment from "@material-ui/core/InputAdornment"
import Link from "@material-ui/core/Link"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import TextField from "@material-ui/core/TextField"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import SearchIcon from "@material-ui/icons/Search"
import useTheme from "@material-ui/styles/useTheme"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import { FormikErrors, useFormik } from "formik"
import { FC, useState } from "react"
import { Link as RouterLink } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { AvatarData } from "../../components/AvatarData/AvatarData"
import { CloseDropdown, OpenDropdown } from "../../components/DropdownArrows/DropdownArrows"
import { EmptyState } from "../../components/EmptyState/EmptyState"
import {
  HelpTooltip,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "../../components/HelpTooltip/HelpTooltip"
import { Margins } from "../../components/Margins/Margins"
import { PageHeader, PageHeaderActions, PageHeaderTitle } from "../../components/PageHeader/PageHeader"
import { Stack } from "../../components/Stack/Stack"
import { TableLoader } from "../../components/TableLoader/TableLoader"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { getDisplayStatus } from "../../util/workspace"

dayjs.extend(relativeTime)

export const Language = {
  createButton: "Create workspace",
  emptyMessage: "Create your first workspace",
  emptyDescription: "Start editing your source code and building your software",
  filterName: "Filters",
  createWorkspaceButton: "Create workspace",
  yourWorkspacesButton: "Your workspaces",
  allWorkspacesButton: "All workspaces",
}

const WorkspaceHelpTooltip: React.FC = () => {
  return (
    <HelpTooltip>
      <HelpTooltipTitle>What is workspace?</HelpTooltipTitle>
      <HelpTooltipText>
        It is your workstation. It is a workspace that will provide you the necessary compute and access to your
        development environment.
      </HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/workspaces.md#create-workspaces">
          Create workspaces
        </HelpTooltipLink>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/workspaces.md#connect-with-ssh">
          Connect with SSH
        </HelpTooltipLink>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/workspaces.md#editors-and-ides">
          Editors and IDEs
        </HelpTooltipLink>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}

interface FilterFormValues {
  query: string
}

export type FilterFormErrors = FormikErrors<FilterFormValues>

export interface WorkspacesPageViewProps {
  loading?: boolean
  workspaces?: TypesGen.Workspace[]
  filter?: string
  onFilter: (query: string) => void
}

export const WorkspacesPageView: FC<WorkspacesPageViewProps> = ({ loading, workspaces, filter, onFilter }) => {
  const styles = useStyles()
  const theme: Theme = useTheme()

  const form = useFormik<FilterFormValues>({
    initialValues: {
      query: filter ?? "",
    },
    onSubmit: ({ query }) => {
      onFilter(query)
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
      <PageHeader>
        <PageHeaderTitle>
          Workspaces
          <WorkspaceHelpTooltip />
        </PageHeaderTitle>

        <PageHeaderActions>
          <Link underline="none" component={RouterLink} to="/workspaces/new">
            <Button startIcon={<AddCircleOutline />} style={{ height: "44px" }}>
              {Language.createWorkspaceButton}
            </Button>
          </Link>
        </PageHeaderActions>
      </PageHeader>

      <Stack direction="row" spacing={0} className={styles.filterContainer}>
        <Button aria-controls="filter-menu" aria-haspopup="true" onClick={handleClick} className={styles.buttonRoot}>
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

      <Table>
        <TableHead>
          <TableRow>
            <TableCell>Name</TableCell>
            <TableCell>Template</TableCell>
            <TableCell>Version</TableCell>
            <TableCell>Last Built</TableCell>
            <TableCell>Status</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {!workspaces && loading && <TableLoader />}
          {workspaces && workspaces.length === 0 && (
            <TableRow>
              <TableCell colSpan={999}>
                <EmptyState
                  message={Language.emptyMessage}
                  description={Language.emptyDescription}
                  cta={
                    <Link underline="none" component={RouterLink} to="/workspaces/new">
                      <Button startIcon={<AddCircleOutline />}>{Language.createButton}</Button>
                    </Link>
                  }
                />
              </TableCell>
            </TableRow>
          )}
          {workspaces &&
            workspaces.map((workspace) => {
              const status = getDisplayStatus(theme, workspace.latest_build)
              return (
                <TableRow key={workspace.id}>
                  <TableCell>
                    <AvatarData
                      title={workspace.name}
                      subtitle={workspace.owner_name}
                      link={`/workspaces/${workspace.id}`}
                    />
                  </TableCell>
                  <TableCell>{workspace.template_name}</TableCell>
                  <TableCell>
                    {workspace.outdated ? (
                      <span style={{ color: theme.palette.error.main }}>outdated</span>
                    ) : (
                      <span style={{ color: theme.palette.text.secondary }}>up to date</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <span data-chromatic="ignore" style={{ color: theme.palette.text.secondary }}>
                      {dayjs().to(dayjs(workspace.latest_build.created_at))}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span style={{ color: status.color }}>{status.status}</span>
                  </TableCell>
                </TableRow>
              )
            })}
        </TableBody>
      </Table>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  welcome: {
    paddingTop: theme.spacing(12),
    paddingBottom: theme.spacing(12),
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    justifyContent: "center",

    "& span": {
      maxWidth: 600,
      textAlign: "center",
      fontSize: theme.spacing(2),
      lineHeight: `${theme.spacing(3)}px`,
    },
  },
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
