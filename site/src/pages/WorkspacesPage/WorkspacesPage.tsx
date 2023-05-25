import { usePagination } from "hooks/usePagination"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import { useWorkspacesData, useWorkspaceUpdate } from "./data"
import { WorkspacesPageView } from "./WorkspacesPageView"
import {
  useFilter,
  useStatusAutocomplete,
  useTemplatesAutocomplete,
  useUsersAutocomplete,
} from "./Filter"
import { useOrganizationId, usePermissions } from "hooks"

const WorkspacesPage: FC = () => {
  const orgId = useOrganizationId()
  const pagination = usePagination()
  const filter = useFilter()
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

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
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
