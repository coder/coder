import Link from "@mui/material/Link"
import { Workspace } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { Maybe } from "components/Conditionals/Maybe"
import { PaginationWidgetBase } from "components/PaginationWidget/PaginationWidgetBase"
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import { Margins } from "components/Margins/Margins"
import {
  PageHeader,
  PageHeaderSubtitle,
  PageHeaderTitle,
} from "components/PageHeader/PageHeader"
import { SearchBarWithFilter } from "components/SearchBarWithFilter/SearchBarWithFilter"
import { Stack } from "components/Stack/Stack"
import { WorkspaceHelpTooltip } from "components/Tooltips"
import { WorkspacesTable } from "components/WorkspacesTable/WorkspacesTable"
import { workspaceFilterQuery } from "utils/filters"
import { useLocalStorage } from "hooks"
import difference from "lodash/difference"

export const Language = {
  pageTitle: "Workspaces",
  yourWorkspacesButton: "Your workspaces",
  allWorkspacesButton: "All workspaces",
  runningWorkspacesButton: "Running workspaces",
  createANewWorkspace: `Create a new workspace from a `,
  template: "Template",
}

const presetFilters = [
  { query: workspaceFilterQuery.me, name: Language.yourWorkspacesButton },
  { query: workspaceFilterQuery.all, name: Language.allWorkspacesButton },
  {
    query: workspaceFilterQuery.running,
    name: Language.runningWorkspacesButton,
  },
  {
    query: workspaceFilterQuery.failed,
    name: "Failed workspaces",
  },
]

export interface WorkspacesPageViewProps {
  error: unknown
  workspaces?: Workspace[]
  count?: number
  page: number
  limit: number
  filter: string
  onPageChange: (page: number) => void
  onFilter: (query: string) => void
  onUpdateWorkspace: (workspace: Workspace) => void
  allowAdvancedScheduling: boolean
  allowWorkspaceActions: boolean
}

export const WorkspacesPageView: FC<
  React.PropsWithChildren<WorkspacesPageViewProps>
> = ({
  workspaces,
  error,
  filter,
  page,
  limit,
  count,
  onFilter,
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
          <AlertBanner
            error={error}
            severity={
              workspaces !== undefined && workspaces.length > 0
                ? "warning"
                : "error"
            }
          />
        </Maybe>
        <Maybe condition={displayImpendingDeletionBanner}>
          <AlertBanner
            severity="info"
            onDismiss={() =>
              saveLocal(
                "dismissedWorkspaceList",
                JSON.stringify(workspaceIdsWithImpendingDeletions),
              )
            }
            dismissible
            text="You have workspaces that will be deleted soon."
          />
        </Maybe>

        <SearchBarWithFilter
          filter={filter}
          onFilter={onFilter}
          presetFilters={presetFilters}
          error={error}
        />
      </Stack>
      <WorkspacesTable
        workspaces={workspaces}
        isUsingFilter={filter !== workspaceFilterQuery.me}
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
