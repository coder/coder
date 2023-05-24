import { usePagination } from "hooks/usePagination"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import { useWorkspacesData, useWorkspaceUpdate } from "./data"
import { WorkspacesPageView } from "./WorkspacesPageView"
import { useFilter } from "./Filter"

const WorkspacesPage: FC = () => {
  const pagination = usePagination()
  const filter = useFilter()
  const { data, error, queryKey } = useWorkspacesData({
    ...pagination,
    query: filter.query,
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
        count={data?.count}
        page={pagination.page}
        limit={pagination.limit}
        filterProps={{ filter }}
        onPageChange={pagination.goToPage}
        onUpdateWorkspace={(workspace) => {
          updateWorkspace.mutate(workspace)
        }}
      />
    </>
  )
}

export default WorkspacesPage
