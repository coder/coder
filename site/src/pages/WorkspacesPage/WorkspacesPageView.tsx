import Link from "@mui/material/Link"
import { Workspace } from "api/typesGenerated"
import { Maybe } from "components/Conditionals/Maybe"
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase"
import { ComponentProps, FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { Stack } from "components/Stack/Stack"
import { WorkspaceHelpTooltip } from "components/Tooltips"
import { WorkspacesTable } from "pages/WorkspacesPage/WorkspacesTable"
import { useLocalStorage } from "hooks"
import { LockedWorkspaceBanner, Count } from "components/WorkspaceDeletion"
import { ErrorAlert } from "components/Alert/ErrorAlert"
import { WorkspacesFilter } from "./filter/filter"
import { hasError, isApiValidationError } from "api/errors"
import {
  PaginationStatus,
  TableToolbar,
} from "components/TableToolbar/TableToolbar"
import Box from "@mui/material/Box"
import Button from "@mui/material/Button"
import DeleteOutlined from "@mui/icons-material/DeleteOutlined"

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
  lockedWorkspaces?: Workspace[]
  checkedWorkspaces: Workspace[]
  count?: number
  filterProps: ComponentProps<typeof WorkspacesFilter>
  page: number
  limit: number
  isWorkspaceBatchActionsEnabled?: boolean
  onPageChange: (page: number) => void
  onUpdateWorkspace: (workspace: Workspace) => void
  onCheckChange: (checkedWorkspaces: Workspace[]) => void
  onDeleteAll: () => void
}

export const WorkspacesPageView: FC<
  React.PropsWithChildren<WorkspacesPageViewProps>
> = ({
  workspaces,
  lockedWorkspaces,
  error,
  limit,
  count,
  filterProps,
  onPageChange,
  onUpdateWorkspace,
  page,
  checkedWorkspaces,
  isWorkspaceBatchActionsEnabled,
  onCheckChange,
  onDeleteAll,
}) => {
  const { saveLocal } = useLocalStorage()

  const workspacesDeletionScheduled = lockedWorkspaces
    ?.filter((workspace) => workspace.deleting_at)
    .map((workspace) => workspace.id)

  const hasLockedWorkspace =
    lockedWorkspaces !== undefined && lockedWorkspaces.length > 0

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
        <Maybe condition={hasError(error) && !isApiValidationError(error)}>
          <ErrorAlert error={error} />
        </Maybe>
        {/* <ImpendingDeletionBanner/> determines its own visibility */}
        <LockedWorkspaceBanner
          workspaces={lockedWorkspaces}
          shouldRedisplayBanner={hasLockedWorkspace}
          onDismiss={() =>
            saveLocal(
              "dismissedWorkspaceList",
              JSON.stringify(workspacesDeletionScheduled),
            )
          }
          count={Count.Multiple}
        />

        <WorkspacesFilter error={error} {...filterProps} />
      </Stack>

      <TableToolbar>
        {checkedWorkspaces.length > 0 ? (
          <>
            <Box>
              Selected <strong>{checkedWorkspaces.length}</strong> of{" "}
              <strong>{workspaces?.length}</strong>{" "}
              {workspaces?.length === 1 ? "workspace" : "workspaces"}
            </Box>

            <Box sx={{ marginLeft: "auto" }}>
              <Button
                size="small"
                startIcon={<DeleteOutlined />}
                onClick={onDeleteAll}
              >
                Delete all
              </Button>
            </Box>
          </>
        ) : (
          <PaginationStatus
            isLoading={!workspaces && !error}
            showing={workspaces?.length ?? 0}
            total={count ?? 0}
            label="workspaces"
          />
        )}
      </TableToolbar>

      <WorkspacesTable
        workspaces={workspaces}
        isUsingFilter={filterProps.filter.used}
        onUpdateWorkspace={onUpdateWorkspace}
        checkedWorkspaces={checkedWorkspaces}
        onCheckChange={onCheckChange}
        isWorkspaceBatchActionsEnabled={isWorkspaceBatchActionsEnabled}
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
