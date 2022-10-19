import { useMachine } from "@xstate/react"
import { DEFAULT_RECORDS_PER_PAGE } from "components/PaginationWidget/PaginationWidget"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useSearchParams } from "react-router-dom"
import { workspaceFilterQuery } from "util/filters"
import { pageTitle } from "util/page"
import { workspacesMachine } from "xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

const WorkspacesPage: FC = () => {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const filter = searchParams.get("filter") ?? workspaceFilterQuery.me
  const currentPage = searchParams.get("page")
    ? Number(searchParams.get("page"))
    : 1
  const [workspacesState, send] = useMachine(workspacesMachine, {
    context: {
      page: currentPage,
      limit: DEFAULT_RECORDS_PER_PAGE,
      filter,
    },
    actions: {
      onPageChange: ({ page }) => {
        navigate({
          search: `?page=${page}`,
        })
      },
    },
  })

  const {
    workspaceRefs,
    count,
    page,
    limit,
    getWorkspacesError,
    getCountError,
  } = workspacesState.context

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        filter={workspacesState.context.filter}
        isLoading={!workspaceRefs}
        workspaceRefs={workspaceRefs}
        count={count}
        getWorkspacesError={getWorkspacesError}
        getCountError={getCountError}
        page={page}
        limit={limit}
        onNext={() => {
          send("NEXT")
        }}
        onPrevious={() => {
          send("PREVIOUS")
        }}
        onGoToPage={(page) => {
          send("GO_TO_PAGE", { page })
        }}
        onFilter={(query) => {
          setSearchParams({ filter: query })
          send({
            type: "UPDATE_FILTER",
            query,
          })
        }}
      />
    </>
  )
}

export default WorkspacesPage
