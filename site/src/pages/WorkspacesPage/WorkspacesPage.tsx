import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { getWorkspaces, updateWorkspaceVersion } from "api/api"
import {
  Workspace,
  WorkspaceBuild,
  WorkspacesResponse,
} from "api/typesGenerated"
import { DEFAULT_RECORDS_PER_PAGE } from "components/PaginationWidget/utils"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useSearchParams } from "react-router-dom"
import { workspaceFilterQuery } from "util/filters"
import { pageTitle } from "util/page"
import { WorkspacesPageView } from "./WorkspacesPageView"

const usePagination = () => {
  const [searchParams, setSearchParams] = useSearchParams()
  const page = searchParams.get("page") ? Number(searchParams.get("page")) : 0
  const limit = DEFAULT_RECORDS_PER_PAGE

  const goToPage = (page: number) => {
    searchParams.set("page", page.toString())
    setSearchParams(searchParams)
  }

  return {
    page,
    limit,
    goToPage,
  }
}

const useFilter = () => {
  const [searchParams, setSearchParams] = useSearchParams()
  const query = searchParams.get("filter") ?? workspaceFilterQuery.me

  const setFilter = (query: string) => {
    searchParams.set("filter", query)
    setSearchParams(searchParams)
  }

  return {
    query,
    setFilter,
  }
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

      return workspace
    }),
  }
}

const WorkspacesPage: FC = () => {
  const filter = useFilter()
  const pagination = usePagination()
  const queryClient = useQueryClient()
  const workspacesQueryKey = ["workspaces", filter.query, pagination.page]
  const { data, error } = useQuery({
    queryKey: workspacesQueryKey,
    queryFn: () =>
      getWorkspaces({
        q: filter.query,
        limit: pagination.limit,
        offset: pagination.page,
      }),
    refetchInterval: 5_000,
  })
  const updateWorkspace = useMutation({
    mutationFn: updateWorkspaceVersion,
    onMutate: async (workspace) => {
      await queryClient.cancelQueries({ queryKey: workspacesQueryKey })
      queryClient.setQueryData<WorkspacesResponse>(
        workspacesQueryKey,
        (oldResponse) => {
          if (oldResponse) {
            return assignPendingStatus(oldResponse, workspace)
          }
        },
      )
    },
    onSuccess: (workspaceBuild) => {
      queryClient.setQueryData<WorkspacesResponse>(
        workspacesQueryKey,
        (oldResponse) => {
          if (oldResponse) {
            return assignLatestBuild(oldResponse, workspaceBuild)
          }
        },
      )
    },
  })

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        workspaces={data?.workspaces}
        error={error}
        filter={filter.query}
        onFilter={filter.setFilter}
        pagination={
          data && {
            limit: pagination.limit,
            page: pagination.page,
            count: data.count,
            onChange: pagination.goToPage,
          }
        }
        onUpdateWorkspace={(workspace) => {
          updateWorkspace.mutate(workspace)
        }}
      />
    </>
  )
}

export default WorkspacesPage
