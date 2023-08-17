import { usePagination } from "hooks/usePagination"
import { Workspace } from "api/typesGenerated"
import {
  useDashboard,
  useIsWorkspaceActionsEnabled,
} from "components/Dashboard/DashboardProvider"
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
import { deleteWorkspace, getWorkspaces } from "api/api"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import Box from "@mui/material/Box"
import { MONOSPACE_FONT_FAMILY } from "theme/constants"
import TextField from "@mui/material/TextField"
import { displayError } from "components/GlobalSnackbar/utils"
import { getErrorMessage } from "api/errors"

const WorkspacesPage: FC = () => {
  const [lockedWorkspaces, setLockedWorkspaces] = useState<Workspace[]>([])
  // If we use a useSearchParams for each hook, the values will not be in sync.
  // So we have to use a single one, centralizing the values, and pass it to
  // each hook.
  const searchParamsResult = useSearchParams()
  const pagination = usePagination({ searchParamsResult })
  const filterProps = useWorkspacesFilter({ searchParamsResult, pagination })
  const { data, error, queryKey, refetch } = useWorkspacesData({
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
  const dashboard = useDashboard()
  const isWorkspaceBatchActionsEnabled =
    dashboard.experiments.includes("workspaces_batch_actions") ||
    process.env.NODE_ENV === "development"

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
        isWorkspaceBatchActionsEnabled={isWorkspaceBatchActionsEnabled}
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

      <BatchDeleteConfirmation
        checkedWorkspaces={checkedWorkspaces}
        open={isDeletingAll}
        onClose={() => {
          setIsDeletingAll(false)
        }}
        onDelete={async () => {
          await refetch()
          setCheckedWorkspaces([])
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

const BatchDeleteConfirmation = ({
  checkedWorkspaces,
  open,
  onClose,
  onDelete,
}: {
  checkedWorkspaces: Workspace[]
  open: boolean
  onClose: () => void
  onDelete: () => void
}) => {
  const [confirmValue, setConfirmValue] = useState("")
  const [confirmError, setConfirmError] = useState(false)
  const [isDeleting, setIsDeleting] = useState(false)

  const close = () => {
    if (isDeleting) {
      return
    }

    onClose()
    setConfirmValue("")
    setConfirmError(false)
    setIsDeleting(false)
  }

  const confirmDeletion = async () => {
    setConfirmError(false)

    if (confirmValue.toLowerCase() !== "delete") {
      setConfirmError(true)
      return
    }

    try {
      setIsDeleting(true)
      await Promise.all(checkedWorkspaces.map((w) => deleteWorkspace(w.id)))
    } catch (e) {
      displayError(
        "Error on deleting workspaces",
        getErrorMessage(e, "An error occurred while deleting the workspaces"),
      )
    } finally {
      close()
      onDelete()
    }
  }

  return (
    <ConfirmDialog
      type="delete"
      open={open}
      confirmLoading={isDeleting}
      onConfirm={confirmDeletion}
      onClose={() => {
        onClose()
        setConfirmValue("")
        setConfirmError(false)
      }}
      title={`Delete ${checkedWorkspaces?.length} ${
        checkedWorkspaces.length === 1 ? "workspace" : "workspaces"
      }`}
      description={
        <form
          onSubmit={async (e) => {
            e.preventDefault()
            await confirmDeletion()
          }}
        >
          <Box>
            Deleting these workspaces is irreversible! Are you sure you want to
            proceed? Type{" "}
            <Box
              component="code"
              sx={{
                fontFamily: MONOSPACE_FONT_FAMILY,
                color: (theme) => theme.palette.text.primary,
                fontWeight: 600,
              }}
            >
              `DELETE`
            </Box>{" "}
            to confirm.
          </Box>
          <TextField
            value={confirmValue}
            required
            autoFocus
            fullWidth
            inputProps={{
              "aria-label": "Type DELETE to confirm",
            }}
            placeholder="Type DELETE to confirm"
            sx={{ mt: 2 }}
            onChange={(e) => {
              setConfirmValue(e.currentTarget.value)
            }}
            error={confirmError}
            helperText={confirmError && "Please type DELETE to confirm"}
          />
        </form>
      }
    />
  )
}
