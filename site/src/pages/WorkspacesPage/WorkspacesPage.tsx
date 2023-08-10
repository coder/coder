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
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"

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
  const [checkedWorkspaces, setCheckedWorkspaces] = useState<Workspace[]>([])
  const [isDeletingAll, setIsDeletingAll] = useState(false)
  const [urlSearchParams] = searchParamsResult

  // We want to uncheck the selected workspaces always when the url changes
  // because of filtering or pagination
  useEffect(() => {
    setCheckedWorkspaces([])
  }, [urlSearchParams])

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        checkedWorkspaces={checkedWorkspaces}
        onCheckChange={setCheckedWorkspaces}
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
        onDeleteAll={() => {
          setIsDeletingAll(true)
        }}
      />

      <ConfirmDialog
        type="delete"
        title={`Delete ${checkedWorkspaces?.length} ${
          checkedWorkspaces.length === 1 ? "workspace" : "workspaces"
        }`}
        description="Deleting these workspaces is irreversible! Are you sure you want to proceed?"
        open={isDeletingAll}
        confirmLoading={false}
        onConfirm={() => {
          alert("DO IT!")
        }}
        onClose={() => {
          setIsDeletingAll(false)
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
