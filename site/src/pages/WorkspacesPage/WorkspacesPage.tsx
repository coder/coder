import { useMachine } from "@xstate/react"
import { FC, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { useSearchParams } from "react-router-dom"
import { workspaceFilterQuery } from "util/filters"
import { pageTitle } from "util/page"
import { workspacesMachine } from "xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

const WorkspacesPage: FC = () => {
  const [workspacesState, send] = useMachine(workspacesMachine)
  const [searchParams, setSearchParams] = useSearchParams()
  const { workspaceRefs } = workspacesState.context

  // On page load, populate the table with workspaces
  useEffect(() => {
    const filter = searchParams.get("filter")
    const query = filter ?? workspaceFilterQuery.me

    send({
      type: "GET_WORKSPACES",
      query,
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

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
