import { useMachine } from "@xstate/react"
import { FC, useEffect } from "react"
import { Helmet } from "react-helmet"
import { useSearchParams } from "react-router-dom"
import { pageTitle } from "../../util/page"
import { workspaceFilterQuery } from "../../util/workspace"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

const WorkspacesPage: FC = () => {
  const [workspacesState, send] = useMachine(workspacesMachine)
  const [_, setSearchParams] = useSearchParams()
  const { workspaceRefs } = workspacesState.context

  // On page load, populate the table with workspaces
  useEffect(() => {
    const query = workspaceFilterQuery.me

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
        loading={workspacesState.hasTag("loading")}
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
