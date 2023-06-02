import { usePagination } from "hooks/usePagination"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import { useWorkspacesData, useWorkspaceUpdate } from "./data"
import { WorkspacesPageView } from "./WorkspacesPageView"
import { useFilter } from "./filter/filter"
import { useOrganizationId, usePermissions } from "hooks"
import {
  useUsersAutocomplete,
  useTemplatesAutocomplete,
  useStatusAutocomplete,
} from "./filter/autocompletes"
import { useSearchParams } from "react-router-dom"
import { useDashboard } from "components/Dashboard/DashboardProvider"

const WorkspacesPage: FC = () => {
  const orgId = useOrganizationId()
  // If we use a useSearchParams for each hook, the values will not be in sync.
  // So we have to use a single one, centralizing the values, and pass it to
  // each hook.
  const searchParamsResult = useSearchParams()
  const pagination = usePagination({ searchParamsResult })
  const filter = useFilter({
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
  const usersAutocomplete = useUsersAutocomplete(
    filter.values.owner,
    (option) => filter.update({ ...filter.values, owner: option?.value }),
    canFilterByUser,
  )
  const templatesAutocomplete = useTemplatesAutocomplete(
    orgId,
    filter.values.template,
    (option) => filter.update({ ...filter.values, template: option?.value }),
  )
  const statusAutocomplete = useStatusAutocomplete(
    filter.values.status,
    (option) => filter.update({ ...filter.values, status: option?.value }),
  )
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
        filterProps={{
          filter,
          autocomplete: {
            users: canFilterByUser ? usersAutocomplete : undefined,
            templates: templatesAutocomplete,
            status: statusAutocomplete,
          },
        }}
        onPageChange={pagination.goToPage}
        onUpdateWorkspace={(workspace) => {
          updateWorkspace.mutate(workspace)
        }}
      />
    </>
  )
}

export default WorkspacesPage
