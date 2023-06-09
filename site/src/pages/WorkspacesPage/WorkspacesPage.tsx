import { usePagination } from "hooks/usePagination"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import { useWorkspacesData, useWorkspaceUpdate } from "./data"
import { WorkspacesPageView } from "./WorkspacesPageView"
import { useOrganizationId, usePermissions } from "hooks"
import { useTemplateFilterMenu, useStatusFilterMenu } from "./filter/menus"
import { useSearchParams } from "react-router-dom"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { useFilter } from "components/Filter/filter"
import { useUserFilterMenu } from "components/Filter/UserFilter"

const WorkspacesPage: FC = () => {
  const orgId = useOrganizationId()
  // If we use a useSearchParams for each hook, the values will not be in sync.
  // So we have to use a single one, centralizing the values, and pass it to
  // each hook.
  const searchParamsResult = useSearchParams()
  const pagination = usePagination({ searchParamsResult })
  const filter = useFilter({
    initialValue: `owner:me`,
    searchParamsResult,
    onUpdate: () => {
      pagination.goToPage(1)
    },
  })
  const { data, error, queryKey } = useWorkspacesData({
    ...pagination,
    query: filter.query,
  })
  const updateWorkspace = useWorkspaceUpdate(queryKey)
  const permissions = usePermissions()
  const canFilterByUser = permissions.viewDeploymentValues
  const userMenu = useUserFilterMenu({
    value: filter.values.owner,
    onChange: (option) =>
      filter.update({ ...filter.values, owner: option?.value }),
    enabled: canFilterByUser,
  })
  const templateMenu = useTemplateFilterMenu({
    orgId,
    value: filter.values.template,
    onChange: (option) =>
      filter.update({ ...filter.values, template: option?.value }),
  })
  const statusMenu = useStatusFilterMenu({
    value: filter.values.status,
    onChange: (option) =>
      filter.update({ ...filter.values, status: option?.value }),
  })
  const dashboard = useDashboard()

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        useNewFilter={dashboard.experiments.includes("workspace_filter")}
        workspaces={data?.workspaces}
        error={error}
        count={data?.count}
        page={pagination.page}
        limit={pagination.limit}
        onPageChange={pagination.goToPage}
        filterProps={{
          filter,
          menus: {
            user: canFilterByUser ? userMenu : undefined,
            template: templateMenu,
            status: statusMenu,
          },
        }}
        onUpdateWorkspace={(workspace) => {
          updateWorkspace.mutate(workspace)
        }}
      />
    </>
  )
}

export default WorkspacesPage
