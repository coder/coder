import { useFilter } from "hooks/useFilter"
import { usePagination } from "hooks/usePagination"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { workspaceFilterQuery } from "utils/filters"
import { pageTitle } from "utils/page"
import { useWorkspacesData, useWorkspaceUpdate } from "./data"
import { WorkspacesPageView } from "./WorkspacesPageView"
import { useQuery } from "@tanstack/react-query"
import { getTemplates, getUsers } from "api/api"
import { useOrganizationId } from "hooks"

const WorkspacesPage: FC = () => {
  const filter = useFilter(workspaceFilterQuery.me)
  const pagination = usePagination()
  const orgId = useOrganizationId()

  const { data, error, queryKey } = useWorkspacesData({
    ...pagination,
    ...filter,
  })
  const updateWorkspace = useWorkspaceUpdate(queryKey)
  const { data: users } = useQuery({
    queryKey: ["users"],
    queryFn: () => getUsers({ limit: 100 }).then((res) => res.users),
  })
  const { data: templates } = useQuery({
    queryKey: ["templates"],
    queryFn: () => getTemplates(orgId),
  })

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
        onPageChange={pagination.goToPage}
        onUpdateWorkspace={(workspace) => {
          updateWorkspace.mutate(workspace)
        }}
        filterProps={{
          query: filter.query,
          onQueryChange: filter.setFilter,
          onLoadTemplates: () => {},
          onLoadUsers: () => {},
          users,
          templates,
        }}
      />
    </>
  )
}

export default WorkspacesPage
