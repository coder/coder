import Link from "@mui/material/Link"
import {
  Template,
  User,
  Workspace,
  WorkspaceStatuses,
} from "api/typesGenerated"
import { Maybe } from "components/Conditionals/Maybe"
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase"
import {
  ComponentProps,
  FC,
  ReactNode,
  forwardRef,
  useRef,
  useState,
} from "react"
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
import { ImpendingDeletionBanner } from "components/WorkspaceDeletion"
import { ErrorAlert } from "components/Alert/ErrorAlert"
import Box from "@mui/material/Box"
import TextField from "@mui/material/TextField"
import { UserAvatar } from "components/UserAvatar/UserAvatar"
import KeyboardArrowDown from "@mui/icons-material/KeyboardArrowDown"
import Button, { ButtonProps } from "@mui/material/Button"
import Menu, { MenuProps } from "@mui/material/Menu"
import MenuItem from "@mui/material/MenuItem"
import SearchOutlined from "@mui/icons-material/SearchOutlined"
import { Avatar, AvatarProps } from "components/Avatar/Avatar"
import InputAdornment from "@mui/material/InputAdornment"
import { Palette, PaletteColor } from "@mui/material/styles"
import IconButton from "@mui/material/IconButton"
import Tooltip from "@mui/material/Tooltip"
import CloseOutlined from "@mui/icons-material/CloseOutlined"
import { getDisplayWorkspaceStatus } from "utils/workspace"
import { Loader } from "components/Loader/Loader"
import MenuList from "@mui/material/MenuList"

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
  onPageChange: (page: number) => void
  onUpdateWorkspace: (workspace: Workspace) => void
  filterProps: ComponentProps<typeof Filter>
}

export const WorkspacesPageView: FC<
  React.PropsWithChildren<WorkspacesPageViewProps>
> = ({
  workspaces,
  error,
  page,
  limit,
  count,
  onPageChange,
  onUpdateWorkspace,
  filterProps,
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
        <ImpendingDeletionBanner
          workspace={workspaces?.find((workspace) => workspace.deleting_at)}
          displayImpendingDeletionBanner={isNewWorkspacesImpendingDeletion()}
          onDismiss={() =>
            saveLocal(
              "dismissedWorkspaceList",
              JSON.stringify(workspaceIdsWithImpendingDeletions),
            )
          }
        />

        <Filter {...filterProps} />
      </Stack>
      <WorkspacesTable
        workspaces={workspaces}
        isUsingFilter={filterProps.query !== ""}
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
  query: string
  onQueryChange: (query: string) => void
  users?: User[]
  onLoadUsers: (query: string) => void
  templates?: Template[]
  onLoadTemplates: (query: string) => void
}> = ({
  query,
  onQueryChange,
  users,
  onLoadUsers,
  templates,
  onLoadTemplates,
}) => {
  const hasFilterQuery = query && query !== ""
  const filterValues = parseFilterQuery(query)

  return (
    <Box display="flex" sx={{ gap: 1, mb: 2 }}>
      <TextField
        sx={{ width: "100%" }}
        color="success"
        size="small"
        InputProps={{
          placeholder: "Search...",
          value: query,
          onChange: (e) => onQueryChange(e.target.value),
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
                <IconButton size="small" onClick={() => onQueryChange("")}>
                  <CloseOutlined sx={{ fontSize: 14 }} />
                </IconButton>
              </Tooltip>
            </InputAdornment>
          ),
        }}
      />
      <OwnerFilter
        users={users}
        onLoad={onLoadUsers}
        value={filterValues.owner}
        onChange={(newOwnerOption) =>
          onQueryChange(
            stringifyFilter({ ...filterValues, owner: newOwnerOption }),
          )
        }
      />
      <TemplateFilter
        templates={templates}
        onLoad={onLoadTemplates}
        value={filterValues.template}
        onChange={(newTemplateOption) =>
          onQueryChange(
            stringifyFilter({ ...filterValues, template: newTemplateOption }),
          )
        }
      />
      <StatusFilter
        value={filterValues.status}
        onChange={(newStatusOption) =>
          onQueryChange(
            stringifyFilter({ ...filterValues, status: newStatusOption }),
          )
        }
      />
    </Box>
  )
}

const OwnerFilter: FC<{
  value: FilterValue["owner"]
  onChange: (value: FilterValue["owner"]) => void
  users?: User[]
  onLoad: (query: string) => void
}> = ({ value, onChange, users }) => {
  const userOptions = users
    ? users.map(
        (user) =>
          ({
            label: user.username,
            value: user.username,
            avatarUrl: user.avatar_url,
          } as UserOption),
      )
    : undefined
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const selectedOption = userOptions
    ? userOptions.find((option) => option.value === value)
    : undefined

  const handleClose = () => {
    setIsMenuOpen(false)
  }

  return (
    <div>
      <MenuButton ref={buttonRef} onClick={() => setIsMenuOpen(true)}>
        User
      </MenuButton>
      <SearchMenu
        id="user-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        onClear={() => {
          onChange(null)
          handleClose()
        }}
        options={userOptions}
        renderOption={(option) => (
          <MenuItem
            key={option.label}
            selected={option.value === selectedOption?.value}
            onClick={() => {
              // if the option selected is already selected, unselect
              onChange(
                selectedOption?.value === option.value ? null : option.value,
              )
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
        )}
      />
    </div>
  )
}

const TemplateFilter: FC<{
  value: FilterValue["template"]
  onChange: (value: FilterValue["template"]) => void
  templates?: Template[]
  onLoad: (query: string) => void
}> = ({ value, onChange, templates }) => {
  const templateOptions = templates
    ? templates.map(
        (template) =>
          ({
            label:
              template.display_name.length > 0
                ? template.display_name
                : template.name,
            value: template.name,
            icon: template.icon,
          } as TemplateOption),
      )
    : undefined
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const selectedOption = templateOptions
    ? templateOptions.find((option) => option.value === value)
    : undefined

  const handleClose = () => {
    setIsMenuOpen(false)
  }

  return (
    <div>
      <MenuButton ref={buttonRef} onClick={() => setIsMenuOpen(true)}>
        Template
      </MenuButton>
      <SearchMenu
        id="template-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
        onClear={() => {
          onChange(null)
          handleClose()
        }}
        options={templateOptions}
        renderOption={(option) => (
          <MenuItem
            key={option.label}
            selected={option.value === selectedOption?.value}
            onClick={() => {
              // if the option selected is already selected, unselect
              onChange(
                selectedOption?.value === option.value ? null : option.value,
              )
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
        )}
      />
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
  const buttonRef = useRef<HTMLButtonElement>(null)
  const [isMenuOpen, setIsMenuOpen] = useState(false)
  const workspaceStatusOptions: WorkspaceStatusOption[] = WorkspaceStatuses.map(
    (status) => {
      const display = getDisplayWorkspaceStatus(status)
      return {
        label: display.text,
        value: status,
        color: display.type ?? "warning",
      }
    },
  )
  const selectedOption = workspaceStatusOptions.find(
    (option) => option.value === value,
  )

  const handleClose = () => {
    setIsMenuOpen(false)
  }

  return (
    <div>
      <MenuButton ref={buttonRef} onClick={() => setIsMenuOpen(true)}>
        Status
      </MenuButton>
      <Menu
        id="status-filter-menu"
        anchorEl={buttonRef.current}
        open={isMenuOpen}
        onClose={handleClose}
      >
        {workspaceStatusOptions.map((option) => (
          <MenuItem
            key={option.label}
            selected={option.value === selectedOption?.value}
            onClick={() => {
              onChange(
                option.value === selectedOption?.value ? null : option.value,
              )
              handleClose()
            }}
          >
            <Box display="flex" alignItems="center" gap={2} fontSize={14}>
              <StatusIndicator option={option} />
              <span>{option.label}</span>
            </Box>
          </MenuItem>
        ))}
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
        ...props.sx,
      }}
    />
  )
})

function SearchMenu<TOption extends { label: string; value: string }>({
  options,
  renderOption,
  onClear,
  ...menuProps
}: Pick<MenuProps, "anchorEl" | "open" | "onClose" | "id"> & {
  options?: TOption[]
  renderOption: (option: TOption) => ReactNode
  onClear: () => void
}) {
  const menuListRef = useRef<HTMLUListElement>(null)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const [searchInputValue, setSearchInputValue] = useState("")
  const visibleOptions = options
    ? options.filter(
        (option) =>
          option.label.toLowerCase().includes(searchInputValue.toLowerCase()) ||
          option.value.toLowerCase().includes(searchInputValue.toLowerCase()),
      )
    : undefined

  return (
    <Menu
      {...menuProps}
      onClose={(event, reason) => {
        menuProps.onClose && menuProps.onClose(event, reason)
        // 250ms is the transition time to close menu
        setTimeout(() => setSearchInputValue(""), 250)
      }}
      sx={{
        "& .MuiPaper-root": {
          width: 320,
          paddingY: 0,
        },
      }}
    >
      <Box
        component="li"
        sx={{
          display: "flex",
          alignItems: "center",
          paddingLeft: 2,
          height: 40,
          borderBottom: (theme) => `1px solid ${theme.palette.divider}`,
        }}
        onKeyDown={(e) => {
          e.stopPropagation()
          if (e.key === "ArrowDown" && menuListRef.current) {
            const firstItem = menuListRef.current.firstChild as HTMLElement
            firstItem.focus()
          }
        }}
      >
        <SearchOutlined
          sx={{
            fontSize: 14,
            color: (theme) => theme.palette.text.secondary,
          }}
        />
        <Box
          tabIndex={-1}
          component="input"
          type="text"
          placeholder="Search..."
          autoFocus
          value={searchInputValue}
          ref={searchInputRef}
          onChange={(e) => {
            setSearchInputValue(e.target.value)
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

      <Box component="li" sx={{ maxHeight: 480, overflowY: "auto" }}>
        <MenuList
          ref={menuListRef}
          onKeyDown={(e) => {
            if (e.shiftKey && e.code === "Tab") {
              e.preventDefault()
              e.stopPropagation()
              searchInputRef.current?.focus()
            }
          }}
        >
          {visibleOptions ? (
            visibleOptions.length > 0 ? (
              visibleOptions.map(renderOption)
            ) : (
              <Box
                sx={{
                  fontSize: 13,
                  color: (theme) => theme.palette.text.secondary,
                  textAlign: "center",
                  py: 1,
                }}
              >
                No results
              </Box>
            )
          ) : (
            <Loader size={14} />
          )}
        </MenuList>
      </Box>
    </Menu>
  )
}

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
