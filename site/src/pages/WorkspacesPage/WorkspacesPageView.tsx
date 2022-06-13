import Button from "@material-ui/core/Button"
import Fade from "@material-ui/core/Fade"
import InputAdornment from "@material-ui/core/InputAdornment"
import Link from "@material-ui/core/Link"
import Menu from "@material-ui/core/Menu"
import MenuItem from "@material-ui/core/MenuItem"
import { fade, makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import TextField from "@material-ui/core/TextField"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import KeyboardArrowRight from "@material-ui/icons/KeyboardArrowRight"
import SearchIcon from "@material-ui/icons/Search"
import useTheme from "@material-ui/styles/useTheme"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import { FormikErrors, useFormik } from "formik"
import { FC, useState } from "react"
import { Link as RouterLink, useNavigate } from "react-router-dom"
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
import { PageHeader, PageHeaderText, PageHeaderTitle } from "../../components/PageHeader/PageHeader"
import { Stack } from "../../components/Stack/Stack"
import { TableLoader } from "../../components/TableLoader/TableLoader"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { getDisplayStatus, workspaceFilterQuery } from "../../util/workspace"

dayjs.extend(relativeTime)

export const Language = {
  createFromTemplateButton: "Create from template",
  emptyCreateWorkspaceMessage: "Create your first workspace",
  emptyCreateWorkspaceDescription: "Start editing your source code and building your software",
  emptyResultsMessage: "No results matched your search",
  filterName: "Filters",
  yourWorkspacesButton: "Your workspaces",
  allWorkspacesButton: "All workspaces",
  workspaceTooltipTitle: "What is a workspace?",
  workspaceTooltipText:
    "A workspace is your development environment in the cloud. It includes the compute infrastructure and tools you need to work on your project",
  workspaceTooltipLink1: "Create workspaces",
  workspaceTooltipLink2: "Connect with SSH",
  workspaceTooltipLink3: "Editors and IDEs",
}

const WorkspaceHelpTooltip: React.FC = () => {
  return (
    <HelpTooltip>
      <HelpTooltipTitle>{Language.workspaceTooltipTitle}</HelpTooltipTitle>
      <HelpTooltipText>{Language.workspaceTooltipText}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/workspaces.md#create-workspaces">
          {Language.workspaceTooltipLink1}
        </HelpTooltipLink>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/workspaces.md#connect-with-ssh">
          {Language.workspaceTooltipLink2}
        </HelpTooltipLink>
        <HelpTooltipLink href="https://github.com/coder/coder/blob/main/docs/workspaces.md#editors-and-ides">
          {Language.workspaceTooltipLink3}
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
  const navigate = useNavigate()
  const theme: Theme = useTheme()

  const form = useFormik<FilterFormValues>({
    enableReinitialize: true,
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
      <PageHeader
        actions={
          <PageHeaderText>
            Create a new workspace from a{" "}
            <Link component={RouterLink} to="/templates">
              template
            </Link>
            .
          </PageHeaderText>
        }
      >
        <PageHeaderTitle>
          <Stack direction="row" spacing={1} alignItems="center">
            <span>Workspaces</span>
            <WorkspaceHelpTooltip />
          </Stack>
        </PageHeaderTitle>
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
            <TableCell width="1%"></TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {!workspaces && loading && <TableLoader />}
          {workspaces && workspaces.length === 0 && (
            <>
              {filter === workspaceFilterQuery.me || filter === workspaceFilterQuery.all ? (
                <TableRow>
                  <TableCell colSpan={999}>
                    <EmptyState
                      message={Language.emptyCreateWorkspaceMessage}
                      description={Language.emptyCreateWorkspaceDescription}
                      cta={
                        <Link underline="none" component={RouterLink} to="/templates">
                          <Button startIcon={<AddCircleOutline />}>{Language.createFromTemplateButton}</Button>
                        </Link>
                      }
                    />
                  </TableCell>
                </TableRow>
              ) : (
                <TableRow>
                  <TableCell colSpan={999}>
                    <EmptyState message={Language.emptyResultsMessage} />
                  </TableCell>
                </TableRow>
              )}
            </>
          )}
          {workspaces &&
            workspaces.map((workspace) => {
              const status = getDisplayStatus(theme, workspace.latest_build)
              const navigateToWorkspacePage = () => {
                navigate(`/@${workspace.owner_name}/${workspace.name}`)
              }
              return (
                <TableRow
                  key={workspace.id}
                  hover
                  data-testid={`workspace-${workspace.id}`}
                  tabIndex={0}
                  onClick={navigateToWorkspacePage}
                  onKeyDown={(event) => {
                    if (event.key === "Enter") {
                      navigateToWorkspacePage()
                    }
                  }}
                  className={styles.clickableTableRow}
                >
                  <TableCell>
                    <AvatarData title={workspace.name} subtitle={workspace.owner_name} />
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
                  <TableCell>
                    <div className={styles.arrowCell}>
                      <KeyboardArrowRight className={styles.arrowRight} />
                    </div>
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
  clickableTableRow: {
    cursor: "pointer",

    "&:hover td": {
      backgroundColor: fade(theme.palette.primary.light, 0.1),
    },

    "&:focus": {
      outline: `1px solid ${theme.palette.secondary.dark}`,
    },

    "& .MuiTableCell-root:last-child": {
      paddingRight: theme.spacing(2),
    },
  },
  arrowRight: {
    color: fade(theme.palette.primary.contrastText, 0.7),
    width: 20,
    height: 20,
  },
  arrowCell: {
    display: "flex",
  },
}))
