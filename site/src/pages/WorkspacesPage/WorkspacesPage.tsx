import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet"
import { pageTitle } from "../../util/page"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"
import { workspaceFilterQuery } from "../../util/workspace"

const WorkspacesPage: FC = () => {
  const [workspacesState, send] = useMachine(workspacesMachine)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspaces")}</title>
      </Helmet>

      <WorkspacesPageView
        filter={workspacesState.context.filter}
        loading={workspacesState.hasTag("loading")}
        workspaces={workspacesState.context.workspaces}
        onFilter={(query) => {
          send({
            type: "SET_FILTER",
            query,
          })
        }}
      />
    </>
  )
}

export default WorkspacesPage
