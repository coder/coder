import { usePagination } from "hooks/usePagination"
import { Workspace } from "api/typesGenerated"
import { useIsWorkspaceActionsEnabled } from "components/Dashboard/DashboardProvider"
import { FC, useEffect, useState } from "react"
import { Helmet } from "react-helmet-async"
import { pageTitle } from "utils/page"
import { useWorkspacesData, useWorkspaceUpdate } from "./data"
import { WorkspacesPageView } from "./WorkspacesPageView"
import { useOrganizationId, usePermissions } from "hooks"
import { useTemplateFilterMenu, useStatusFilterMenu } from "./filter/menus"
import { useSearchParams } from "react-router-dom"
import { useFilter } from "components/Filter/filter"
import { useUserFilterMenu } from "components/Filter/UserFilter"
import { getWorkspaces } from "api/api"

const WorkspacesPage: FC = () => {
  const [lockedWorkspaces, setLockedWorkspaces] = useState<Workspace[]>([])
  // If we use a useSearchParams for each hook, the values will not be in sync.
  // So we have to use a single one, centralizing the values, and pass it to
  // each hook.
  const searchParamsResult = useSearchParams()
  const pagination = usePagination({ searchParamsResult })
  const filterProps = useWorkspacesFilter({ searchParamsResult, pagination })
  const { data, error, queryKey } = useWorkspacesData({
    ...pagination,
    query: filterProps.filter.query,
  })

  const experimentEnabled = useIsWorkspaceActionsEnabled()
  // If workspace actions are enabled we need to fetch the locked
  // workspaces as well. This lets us determine whether we should
  // show a banner to the user indicating that some of their workspaces
  // are at risk of being deleted.
  useEffect(() => {
    if (experimentEnabled) {
      const includesLocked = filterProps.filter.query.includes("locked_at")
      const lockedQuery = includesLocked
        ? filterProps.filter.query
        : filterProps.filter.query + " locked_at:1970-01-01"

      if (includesLocked && data) {
        setLockedWorkspaces(data.workspaces)
      } else {
        getWorkspaces({ q: lockedQuery })
          .then((resp) => {
            setLockedWorkspaces(resp.workspaces)
          })
          .catch(() => {
            // TODO
          })
      }
    } else {
      // If the experiment isn't included then we'll pretend
      // like locked workspaces don't exist.
      setLockedWorkspaces([])
    }
  }, [experimentEnabled, data, filterProps.filter.query])

  const updateWorkspace = useWorkspaceUpdate(queryKey)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        workspaces={data?.workspaces}
        lockedWorkspaces={lockedWorkspaces}
        error={error}
        count={data?.count}
        page={pagination.page}
        limit={pagination.limit}
        onPageChange={pagination.goToPage}
        filterProps={filterProps}
        onUpdateWorkspace={(workspace) => {
          updateWorkspace.mutate(workspace)
        }}
      />
    </>
  )
}

export default WorkspacesPage

type UseWorkspacesFilterOptions = {
  searchParamsResult: ReturnType<typeof useSearchParams>
  pagination: ReturnType<typeof usePagination>
}

const useWorkspacesFilter = ({
  searchParamsResult,
  pagination,
}: UseWorkspacesFilterOptions) => {
  const orgId = useOrganizationId()
  const filter = useFilter({
    initialValue: `owner:me`,
    searchParamsResult,
    onUpdate: () => {
      pagination.goToPage(1)
    },
  })
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

  return {
    filter,
    menus: {
      user: canFilterByUser ? userMenu : undefined,
      template: templateMenu,
      status: statusMenu,
    },
  }
}
