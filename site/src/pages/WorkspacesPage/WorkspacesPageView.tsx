import Link from "@mui/material/Link"
import { ProvisionerJobStatus, Workspace } from "api/typesGenerated"
import { Alert } from "components/Alert/Alert"
import { Maybe } from "components/Conditionals/Maybe"
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase"
import { FC, forwardRef, useRef, useState } from "react"
import { Link as RouterLink } from "react-router-dom"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { WorkspaceHelpTooltip } from "components/Tooltips"
import { WorkspacesTable } from "components/WorkspacesTable/WorkspacesTable"
import { useLocalStorage } from "hooks"
import difference from "lodash/difference"
import { ErrorAlert } from "components/Alert/ErrorAlert"
import Box from "@mui/material/Box"
import TextField from "@mui/material/TextField"
import { MockTemplate, MockUser, MockUser2 } from "testHelpers/entities"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown"
import { getDisplayJobStatus, jobStatuses } from "utils/workspace"
import { useTheme } from "@mui/styles"
import Button, { ButtonProps } from "@mui/material/Button"
import Menu from "@mui/material/Menu"
import MenuItem from "@mui/material/MenuItem"
import SearchOutlined from "@mui/icons-material/SearchOutlined"
import { Avatar, AvatarProps } from "components/Avatar/Avatar"
import InputAdornment from "@mui/material/InputAdornment"
import Divider from "@mui/material/Divider"
import { Palette, PaletteColor } from "@mui/material/styles"
import IconButton from "@mui/material/IconButton"
import Tooltip from "@mui/material/Tooltip"
import CloseOutlined from "@mui/icons-material/CloseOutlined"

export const Language = {
  pageTitle: "Workspaces",
  yourWorkspacesButton: "Your workspaces",
  allWorkspacesButton: "All workspaces",
  runningWorkspacesButton: "Running workspaces",
  createANewWorkspace: `Create a new workspace from a `,
  template: "Template",
}

export interface WorkspacesPageViewProps {
  error: unknown
  workspaces?: Workspace[]
  count?: number
  page: number
  limit: number
  filterQuery: string
  onPageChange: (page: number) => void
  onFilterQueryChange: (query: string) => void
  onUpdateWorkspace: (workspace: Workspace) => void
  allowAdvancedScheduling: boolean
  allowWorkspaceActions: boolean
}

export const WorkspacesPageView: FC<
  React.PropsWithChildren<WorkspacesPageViewProps>
> = ({
  workspaces,
  error,
  filterQuery,
  page,
  limit,
  count,
  onFilterQueryChange,
  onPageChange,
  onUpdateWorkspace,
  allowAdvancedScheduling,
  allowWorkspaceActions,
}) => {
  const { saveLocal, getLocal } = useLocalStorage()

  const workspaceIdsWithImpendingDeletions = workspaces
    ?.filter((workspace) => workspace.deleting_at)
    .map((workspace) => workspace.id)

  /**
   * Returns a boolean indicating if there are workspaces that have been
   * recently marked for deletion but are not in local storage.
   * If there are, we want to alert the user so they can potentially take action
   * before deletion takes place.
   * @returns {boolean}
   */
  const isNewWorkspacesImpendingDeletion = (): boolean => {
    const dismissedList = getLocal("dismissedWorkspaceList")
    if (!dismissedList) {
      return true
    }

    const diff = difference(
      workspaceIdsWithImpendingDeletions,
      JSON.parse(dismissedList),
    )

    return diff && diff.length > 0
  }

  const displayImpendingDeletionBanner =
    (allowAdvancedScheduling &&
      allowWorkspaceActions &&
      workspaceIdsWithImpendingDeletions &&
      workspaceIdsWithImpendingDeletions.length > 0 &&
      isNewWorkspacesImpendingDeletion()) ??
    false

  return (
    <Margins>
      <PageHeader>
        <PageHeaderTitle>
          <Stack direction="row" spacing={1} alignItems="center">
            <span>{Language.pageTitle}</span>
            <WorkspaceHelpTooltip />
          </Stack>
        </PageHeaderTitle>

        <PageHeaderSubtitle>
          {Language.createANewWorkspace}
          <Link component={RouterLink} to="/templates">
            {Language.template}
          </Link>
          .
        </PageHeaderSubtitle>
      </PageHeader>

      <Stack>
        <Maybe condition={Boolean(error)}>
          <ErrorAlert error={error} />
        </Maybe>
        <Maybe condition={displayImpendingDeletionBanner}>
          <Alert
            severity="info"
            onDismiss={() =>
              saveLocal(
                "dismissedWorkspaceList",
                JSON.stringify(workspaceIdsWithImpendingDeletions),
              )
            }
            dismissible
          >
            You have workspaces that will be deleted soon.
          </Alert>
        </Maybe>

        <Filter
          filterQuery={filterQuery}
          onFilterQueryChange={onFilterQueryChange}
        />
      </Stack>
      <WorkspacesTable
        workspaces={workspaces}
        isUsingFilter={false}
        //isUsingFilter={filter !== workspaceFilterQuery.me}
        onUpdateWorkspace={onUpdateWorkspace}
        error={error}
      />
      {count !== undefined && (
        <PaginationWidgetBase
          count={count}
          limit={limit}
          onChange={onPageChange}
          page={page}
        />
      )}
    </Margins>
  )
}

type UserOption = {
  label: string
  value: string
  avatarUrl?: string
}
type WorkspaceStatusOption = {
  label: string
  value: string
  color: string
}
type TemplateOption = {
  label: string
  value: string
  icon?: string
}
// There are null values because of the Autocomplete onChange API
export type FilterValue = {
  owner?: UserOption["value"] | null
  status?: WorkspaceStatusOption["value"] | null
  template?: TemplateOption["value"] | null
}

const Filter: FC<{
  filterQuery: string
  onFilterQueryChange: (filterQuery: string) => void
}> = ({ filterQuery, onFilterQueryChange }) => {
  const hasFilterQuery = filterQuery && filterQuery !== ""
  const filter = parseFilterQuery(filterQuery)

  return (
    <Box display="flex" sx={{ gap: 1, mb: 2 }}>
      <TextField
        sx={{ width: "100%" }}
        color="success"
        size="small"
        InputProps={{
          placeholder: "Search...",
          value: filterQuery,
          onChange: (e) => onFilterQueryChange(e.target.value),
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
              <Tooltip title="Clear search">
                <IconButton
                  size="small"
                  onClick={() => onFilterQueryChange("")}
                >
                  <CloseOutlined sx={{ fontSize: 14 }} />
                </IconButton>
              </Tooltip>
            </InputAdornment>
          ),
        }}
      />
      <OwnerFilter
        value={filter.owner}
        onChange={(newOwnerOption) =>
          onFilterQueryChange(
            stringifyFilter({ ...filter, owner: newOwnerOption }),
          )
        }
      />
      <TemplateFilter
        value={filter.template}
        onChange={(newTemplateOption) =>
          onFilterQueryChange(
            stringifyFilter({ ...filter, template: newTemplateOption }),
          )
        }
      />
      <StatusFilter
        value={filter.status}
        onChange={(newStatusOption) =>
          onFilterQueryChange(
            stringifyFilter({ ...filter, status: newStatusOption }),
          )
        }
      />
    </Box>
  )
}

const OwnerFilter: FC<{
  value: FilterValue["owner"]
  onChange: (value: FilterValue["owner"]) => void
}> = ({ value, onChange }) => {
  const userOptions = [MockUser, MockUser2].map(
    (user) =>
      ({
        label: user.username,
        value: user.username,
        avatarUrl: user.avatar_url,
      } as UserOption),
  )
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const selectedOption = userOptions.find((option) => option.value === value)

  const handleClose = () => {
    setIsMenuOpen(false)
  }

  return (
    <div>
      <MenuButton
        ref={buttonRef}
        startIcon={
          selectedOption ? (
            <UserAvatar
              username={selectedOption.label}
              avatarURL={selectedOption.avatarUrl}
              sx={{ width: 16, height: 16, fontSize: "8px !important" }}
            />
          ) : undefined
        }
        onClick={() => setIsMenuOpen(true)}
        sx={{ width: 220 }}
      >
        {selectedOption ? selectedOption.label : "All users"}
      </MenuButton>
      <Menu
        id="user-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        sx={{ "& .MuiPaper-root": { width: 220 } }}
      >
        <Box
          component="li"
          sx={{
            display: "flex",
            alignItems: "center",
            paddingLeft: 2,
            height: 36,
          }}
        >
          <SearchOutlined
            sx={{
              fontSize: 14,
              color: (theme) => theme.palette.text.secondary,
            }}
          />
          <Box
            component="input"
            type="text"
            placeholder="Search..."
            autoFocus
            onKeyDown={(e) => {
              e.stopPropagation()
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
        <Divider />
        {userOptions.map((option) => (
          <MenuItem
            key={option.label}
            selected={option.value === selectedOption?.value}
            onClick={() => {
              onChange(option.value)
              handleClose()
            }}
          >
            <Box display="flex" alignItems="center" gap={2} fontSize={14}>
              <UserAvatar
                username={option.label}
                avatarURL={option.avatarUrl}
                sx={{ width: 16, height: 16, fontSize: 8 }}
              />
              <span>{option.label}</span>
            </Box>
          </MenuItem>
        ))}
        <Divider />
        <MenuItem
          onClick={() => {
            onChange(null)
            handleClose()
          }}
          sx={{ fontSize: 14 }}
        >
          All users
        </MenuItem>
      </Menu>
    </div>
  )
}

const TemplateFilter: FC<{
  value: FilterValue["template"]
  onChange: (value: FilterValue["template"]) => void
}> = ({ value, onChange }) => {
  const templateOptions = [MockTemplate].map(
    (template) =>
      ({
        label: template.display_name ?? template.name,
        value: template.name,
        icon: template.icon,
      } as TemplateOption),
  )
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const selectedOption = templateOptions.find(
    (option) => option.value === value,
  )

  const handleClose = () => {
    setIsMenuOpen(false)
  }

  return (
    <div>
      <MenuButton
        ref={buttonRef}
        startIcon={
          selectedOption ? (
            <TemplateAvatar
              sx={{ width: 14, height: 14, fontSize: 8 }}
              templateName={selectedOption.label}
              icon={selectedOption.icon}
            />
          ) : undefined
        }
        onClick={() => setIsMenuOpen(true)}
        sx={{ width: 220 }}
      >
        {selectedOption ? selectedOption.label : "All templates"}
      </MenuButton>
      <Menu
        id="template-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        sx={{ "& .MuiPaper-root": { width: 220 } }}
      >
        <Box
          component="li"
          sx={{
            display: "flex",
            alignItems: "center",
            paddingLeft: 2,
            height: 36,
          }}
        >
          <SearchOutlined
            sx={{
              fontSize: 14,
              color: (theme) => theme.palette.text.secondary,
            }}
          />
          <Box
            component="input"
            type="text"
            placeholder="Search..."
            autoFocus
            onKeyDown={(e) => {
              e.stopPropagation()
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
        <Divider />
        {templateOptions.map((option) => (
          <MenuItem
            key={option.label}
            selected={option.value === selectedOption?.value}
            onClick={() => {
              onChange(option.value)
              handleClose()
            }}
          >
            <Box display="flex" alignItems="center" gap={2} fontSize={14}>
              <TemplateAvatar
                templateName={option.label}
                icon={option.icon}
                sx={{ width: 14, height: 14, fontSize: 8 }}
              />
              <span>{option.label}</span>
            </Box>
          </MenuItem>
        ))}
        <Divider />
        <MenuItem
          onClick={() => {
            onChange(null)
            handleClose()
          }}
          sx={{ fontSize: 14 }}
        >
          All templates
        </MenuItem>
      </Menu>
    </div>
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

const StatusFilter: FC<{
  value: FilterValue["status"]
  onChange: (value: FilterValue["status"]) => void
}> = ({ value, onChange }) => {
  const theme = useTheme()
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const workspaceStatusOptions: WorkspaceStatusOption[] = Object.keys(
    jobStatuses,
  ).map((status) => {
    const display = getDisplayJobStatus(theme, status as ProvisionerJobStatus)
    return {
      label: display.status,
      value: status,
      color: display.type,
    }
  })
  const selectedOption = workspaceStatusOptions.find(
    (option) => option.value === value,
  )

  const handleClose = () => {
    setIsMenuOpen(false)
  }

  return (
    <div>
      <MenuButton
        ref={buttonRef}
        startIcon={
          selectedOption ? (
            <StatusIndicator option={selectedOption} />
          ) : undefined
        }
        onClick={() => setIsMenuOpen(true)}
        sx={{ width: 140 }}
      >
        {selectedOption ? selectedOption.label : "All statuses"}
      </MenuButton>
      <Menu
        id="status-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        sx={{ "& .MuiPaper-root": { width: 140 } }}
      >
        {workspaceStatusOptions.map((option) => (
          <MenuItem
            key={option.label}
            selected={option.label === selectedOption?.label}
            onClick={() => {
              onChange(option.value)
              handleClose()
            }}
          >
            <Box display="flex" alignItems="center" gap={2} fontSize={14}>
              <StatusIndicator option={option} />
              <span>{option.label}</span>
            </Box>
          </MenuItem>
        ))}
        <Divider />
        <MenuItem
          onClick={() => {
            onChange(null)
            handleClose()
          }}
          sx={{ fontSize: 14 }}
        >
          All statuses
        </MenuItem>
      </Menu>
    </div>
  )
}

const StatusIndicator: FC<{ option: WorkspaceStatusOption }> = ({ option }) => {
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

const MenuButton = forwardRef<HTMLButtonElement, ButtonProps>((props, ref) => {
  return (
    <Button
      ref={ref}
      endIcon={<KeyboardArrowDown />}
      {...props}
      sx={{
        borderRadius: "6px",
        lineHeight: 0,
        "& .MuiButton-endIcon": { marginLeft: "auto" },
        ...props.sx,
      }}
    />
  )
})

const parseFilterQuery = (filterQuery: string): FilterValue => {
  const pairs = filterQuery.split(" ")
  const result: FilterValue = {}

  for (const pair of pairs) {
    const [key, value] = pair.split(":") as [
      keyof FilterValue,
      string | undefined,
    ]
    result[key] = value ?? null
  }

  return result
}

const stringifyFilter = (filterValue: FilterValue): string => {
  let result = ""

  for (const key in filterValue) {
    const value = filterValue[key as keyof FilterValue]
    if (value !== null) {
      result += `${key}:${value} `
    }
  }

  return result.trim()
}
