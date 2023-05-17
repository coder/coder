import { useFilter } from "hooks/useFilter"
import { usePagination } from "hooks/usePagination"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { workspaceFilterQuery } from "utils/filters"
import { pageTitle } from "utils/page"
import { useWorkspacesData, useWorkspaceUpdate } from "./data"
import { WorkspacesPageView } from "./WorkspacesPageView"
import { useDashboard } from "components/Dashboard/DashboardProvider"

const WorkspacesPage: FC = () => {
  const filter = useFilter(workspaceFilterQuery.me)
  const pagination = usePagination()
  const { entitlements, experiments } = useDashboard()
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions")

  const { data, error, queryKey } = useWorkspacesData({
    ...pagination,
    ...filter,
  })
  const updateWorkspace = useWorkspaceUpdate(queryKey)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        workspaces={data?.workspaces}
        error={error}
        filter={filter.query}
        onFilter={filter.setFilter}
        count={data?.count}
        page={pagination.page}
        limit={pagination.limit}
        onPageChange={pagination.goToPage}
        onUpdateWorkspace={(workspace) => {
          updateWorkspace.mutate(workspace)
        }}
        allowAdvancedScheduling={allowAdvancedScheduling}
        allowWorkspaceActions={allowWorkspaceActions}
      />
    </>
  )
}

export default WorkspacesPage
