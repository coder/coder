import { usePagination } from "hooks/usePagination"
import { Workspace } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"
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
import { getWorkspaces, updateWorkspaceVersion } from "api/api"

const WorkspacesPage: FC = () => {
  const orgId = useOrganizationId()
  const [lockedWorkspaces, setLockedWorkspaces] = useState<Workspace[]>([])
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

  const { entitlements, experiments } = useDashboard()
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions")

  if (allowWorkspaceActions && allowAdvancedScheduling) {
    const includesLocked = filter.query.includes("locked_at")
    const lockedQuery = includesLocked
      ? filter.query
      : filter.query + " locked_at:1970-01-01"

    useEffect(() => {
      if (includesLocked && data) {
        setLockedWorkspaces(data.workspaces)
      } else {
        getWorkspaces({ q: lockedQuery })
          .then((resp) => {
            setLockedWorkspaces(resp.workspaces)
          })
          .catch((err) => {
            console.log(err)
          })
      }
    })
  } else {
    // If the experiment isn't included then we'll pretend
    // like locked workspaces don't exist.
    setLockedWorkspaces([])
  }

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
