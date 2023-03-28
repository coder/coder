import {
  QueryKey,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"
import { getWorkspaces, updateWorkspaceVersion } from "api/api"
import { getErrorMessage } from "api/errors"
import {
  Workspace,
  WorkspaceBuild,
  WorkspacesResponse,
} from "api/typesGenerated"
import { displayError } from "components/GlobalSnackbar/utils"
import { useTranslation } from "react-i18next"

type UseWorkspacesDataParams = {
  page: number
  limit: number
  query: string
}

export const useWorkspacesData = ({
  page,
  limit,
  query,
}: UseWorkspacesDataParams) => {
  const queryKey = ["workspaces", query, page]
  const result = useQuery({
    queryKey,
    queryFn: () =>
      getWorkspaces({
        q: query,
        limit: limit,
        // If the page is <= 0, just use offset 0. This usually happens
        // if the page is not provided.
        offset: page <= 0 ? 0 : (page - 1) * limit,
      }),
    refetchInterval: 5_000,
  })

  return {
    ...result,
    queryKey,
  }
}

export const useWorkspaceUpdate = (queryKey: QueryKey) => {
  const queryClient = useQueryClient()
  const { t } = useTranslation("workspacesPage")

  return useMutation({
    mutationFn: updateWorkspaceVersion,
    onMutate: async (workspace) => {
      await queryClient.cancelQueries({ queryKey })
      queryClient.setQueryData<WorkspacesResponse>(queryKey, (oldResponse) => {
        if (oldResponse) {
          return assignPendingStatus(oldResponse, workspace)
        }
      })
    },
    onSuccess: (workspaceBuild) => {
      queryClient.setQueryData<WorkspacesResponse>(queryKey, (oldResponse) => {
        if (oldResponse) {
          return assignLatestBuild(oldResponse, workspaceBuild)
        }
      })
    },
    onError: (error) => {
      const message = getErrorMessage(error, t("updateVersionError"))
      displayError(message)
    },
  })
}

const assignLatestBuild = (
  oldResponse: WorkspacesResponse,
  build: WorkspaceBuild,
): WorkspacesResponse => {
  return {
    ...oldResponse,
    workspaces: oldResponse.workspaces.map((workspace) => {
      if (workspace.id === build.workspace_id) {
        return {
          ...workspace,
          latest_build: build,
        }
      }

      return workspace
    }),
  }
}

const assignPendingStatus = (
  oldResponse: WorkspacesResponse,
  workspace: Workspace,
): WorkspacesResponse => {
  return {
    ...oldResponse,
    workspaces: oldResponse.workspaces.map((workspaceItem) => {
      if (workspaceItem.id === workspace.id) {
        return {
          ...workspace,
          latest_build: {
            ...workspace.latest_build,
            status: "pending",
            job: {
              ...workspace.latest_build.job,
              status: "pending",
            },
          },
        } as Workspace
      }

      return workspaceItem
    }),
  }
}
