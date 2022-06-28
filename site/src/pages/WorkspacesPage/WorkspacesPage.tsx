import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet"
import { useSearchParams } from "react-router-dom"
import { pageTitle } from "../../util/page"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

const WorkspacesPage: FC = () => {
  const [workspacesState, send] = useMachine(workspacesMachine)
  const [_, setSearchParams] = useSearchParams()
  const { workspaceRefs } = workspacesState.context

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
