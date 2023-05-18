import { useFilter } from "hooks/useFilter"
import { usePagination } from "hooks/usePagination"
import { FC, useEffect, useState } from "react"
import { Helmet } from "react-helmet-async"
import { workspaceFilterQuery } from "utils/filters"
import { pageTitle } from "utils/page"
import { useWorkspacesData, useWorkspaceUpdate } from "./data"
import { WorkspacesPageView } from "./WorkspacesPageView"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { Workspace } from "api/typesGenerated"

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

  const [displayedWorkspaces, setDisplayedWorkspaces] = useState<Workspace[]>(
    [],
  )

  useEffect(() => {
    const fetchedWorkspaces = data?.workspaces || []
    if (fetchedWorkspaces) {
      if (displayedWorkspaces.length === 0) {
        setDisplayedWorkspaces(fetchedWorkspaces)
      } else {
        // Merge the fetched workspaces with the displayed onws, without changing the order of the existing items
        const mergedItems = displayedWorkspaces
          .map((item) => {
            const fetchedItem = fetchedWorkspaces.find(
              (fItem) => fItem.id === item.id,
            )

            if (!fetchedItem) {
              return null
            }

            // If the fetched item already exists, update its data without changing its position
            return { ...item, ...fetchedItem }
          })
          .filter((item) => item !== null) // Remove the removed items (null values)

        // Add new items to the beginning of the list
        fetchedWorkspaces.forEach((fetchedItem) => {
          if (!mergedItems.some((item) => item?.id === fetchedItem.id)) {
            mergedItems.unshift(fetchedItem)
          }
        })

        setDisplayedWorkspaces(
          mergedItems.filter((item) => item !== null) as Workspace[],
        )
      }
    }
  }, [data?.workspaces, displayedWorkspaces])

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        workspaces={data && displayedWorkspaces}
        error={error}
        filter={filter.query}
        onFilter={filter.setFilter}
        count={displayedWorkspaces.length}
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
