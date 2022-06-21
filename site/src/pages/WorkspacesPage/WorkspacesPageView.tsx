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
import RefreshIcon from "@material-ui/icons/Refresh"
import SearchIcon from "@material-ui/icons/Search"
import useTheme from "@material-ui/styles/useTheme"
import { useActor } from "@xstate/react"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import { FormikErrors, useFormik } from "formik"
import { FC, useState } from "react"
import { Link as RouterLink, useNavigate } from "react-router-dom"
import { AvatarData } from "../../components/AvatarData/AvatarData"
import { CloseDropdown, OpenDropdown } from "../../components/DropdownArrows/DropdownArrows"
import { EmptyState } from "../../components/EmptyState/EmptyState"
import { Margins } from "../../components/Margins/Margins"
import { PageHeader, PageHeaderSubtitle, PageHeaderTitle } from "../../components/PageHeader/PageHeader"
import { Stack } from "../../components/Stack/Stack"
import { TableCellLink } from "../../components/TableCellLink/TableCellLink"
import { TableLoader } from "../../components/TableLoader/TableLoader"
import {
  HelpTooltip,
  HelpTooltipAction,
  HelpTooltipLink,
  HelpTooltipLinksGroup,
  HelpTooltipText,
  HelpTooltipTitle,
} from "../../components/Tooltips/HelpTooltip/HelpTooltip"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { getDisplayStatus, workspaceFilterQuery } from "../../util/workspace"
import { WorkspaceItemMachineRef } from "../../xServices/workspaces/workspacesXService"

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
    "A workspace is your development environment in the cloud. It includes the infrastructure and tools you need to work on your project.",
  workspaceTooltipLink1: "Create workspaces",
  workspaceTooltipLink2: "Connect with SSH",
  workspaceTooltipLink3: "Editors and IDEs",
  outdatedLabel: "Outdated",
  upToDateLabel: "Up to date",
  versionTooltipText: "This workspace version is outdated and a newer version is available.",
  updateVersionLabel: "Update version",
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

const OutdatedHelpTooltip: React.FC<{ onUpdateVersion: () => void }> = ({ onUpdateVersion }) => {
  return (
    <HelpTooltip size="small">
      <HelpTooltipTitle>{Language.outdatedLabel}</HelpTooltipTitle>
      <HelpTooltipText>{Language.versionTooltipText}</HelpTooltipText>
      <HelpTooltipLinksGroup>
        <HelpTooltipAction icon={RefreshIcon} onClick={onUpdateVersion}>
          {Language.updateVersionLabel}
        </HelpTooltipAction>
      </HelpTooltipLinksGroup>
    </HelpTooltip>
  )
}

const WorkspaceRow: React.FC<{ workspaceRef: WorkspaceItemMachineRef }> = ({ workspaceRef }) => {
  const styles = useStyles()
  const navigate = useNavigate()
  const theme: Theme = useTheme()
  const [workspaceState, send] = useActor(workspaceRef)
  const { data: workspace } = workspaceState.context
  const status = getDisplayStatus(theme, workspace.latest_build)
  const workspacePageLink = `/@${workspace.owner_name}/${workspace.name}`

  return (
    <TableRow
      hover
      data-testid={`workspace-${workspace.id}`}
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          navigate(workspacePageLink)
        }
      }}
      className={styles.clickableTableRow}
    >
      <TableCellLink to={workspacePageLink}>
        <AvatarData title={workspace.name} subtitle={workspace.owner_name} />
      </TableCellLink>
      <TableCellLink to={workspacePageLink}>{workspace.template_name}</TableCellLink>
      <TableCellLink to={workspacePageLink}>
        {workspace.outdated ? (
          <span className={styles.outdatedLabel}>
            {Language.outdatedLabel}
            <OutdatedHelpTooltip
              onUpdateVersion={() => {
                send("UPDATE_VERSION")
              }}
            />
          </span>
        ) : (
          <span style={{ color: theme.palette.text.secondary }}>{Language.upToDateLabel}</span>
        )}
      </TableCellLink>
      <TableCellLink to={workspacePageLink}>
        <span data-chromatic="ignore" style={{ color: theme.palette.text.secondary }}>
          {dayjs().to(dayjs(workspace.latest_build.created_at))}
        </span>
      </TableCellLink>
      <TableCellLink to={workspacePageLink}>
        <span style={{ color: status.color }}>{status.status}</span>
      </TableCellLink>
      <TableCellLink to={workspacePageLink}>
        <div className={styles.arrowCell}>
          <KeyboardArrowRight className={styles.arrowRight} />
        </div>
      </TableCellLink>
    </TableRow>
  )
}

interface FilterFormValues {
  query: string
}

export type FilterFormErrors = FormikErrors<FilterFormValues>

export interface WorkspacesPageViewProps {
  loading?: boolean
  workspaceRefs?: WorkspaceItemMachineRef[]
  filter?: string
  onFilter: (query: string) => void
}

export const WorkspacesPageView: FC<WorkspacesPageViewProps> = ({ loading, workspaceRefs, filter, onFilter }) => {
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
          <Stack direction="row" spacing={1} alignItems="center">
            <span>Workspaces</span>
            <WorkspaceHelpTooltip />
          </Stack>
        </PageHeaderTitle>

        <PageHeaderSubtitle>
          Create a new workspace from a{" "}
          <Link component={RouterLink} to="/templates">
            Template
          </Link>
          .
        </PageHeaderSubtitle>
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
            <TableCell width="35%">Name</TableCell>
            <TableCell width="15%">Template</TableCell>
            <TableCell width="15%">Version</TableCell>
            <TableCell width="20%">Last Built</TableCell>
            <TableCell width="15%">Status</TableCell>
            <TableCell width="1%"></TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {!workspaceRefs && loading && <TableLoader />}
          {workspaceRefs && workspaceRefs.length === 0 && (
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
          {workspaceRefs &&
            workspaceRefs.map((workspaceRef) => <WorkspaceRow workspaceRef={workspaceRef} key={workspaceRef.id} />)}
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
  outdatedLabel: {
    color: theme.palette.error.main,
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(0.5),
  },
}))
