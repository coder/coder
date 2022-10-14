import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useSearchParams } from "react-router-dom"
import { workspaceFilterQuery } from "util/filters"
import { pageTitle } from "util/page"
import { workspacesMachine } from "xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

const WorkspacesPage: FC = () => {
  const [searchParams, setSearchParams] = useSearchParams()
  const filter = searchParams.get("filter") ?? workspaceFilterQuery.me
  const [workspacesState, send] = useMachine(workspacesMachine, {
    context: {
      filter,
    },
  })

  const { workspaceRefs } = workspacesState.context

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        filter={workspacesState.context.filter}
        isLoading={!workspaceRefs}
        workspaceRefs={workspaceRefs}
        onFilter={(query) => {
          setSearchParams({ filter: query })
          send({
            type: "GET_WORKSPACES",
            query,
          })
        }}
      />
    </>
  )
}

export default WorkspacesPage
