import { useMachine } from "@xstate/react"
import { FC } from "react"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

const WorkspacesPage: FC = () => {
  const [workspacesState] = useMachine(workspacesMachine)

  return (
    <>
      <WorkspacesPageView
        loading={workspacesState.hasTag("loading")}
        workspaces={workspacesState.context.workspaces}
        error={workspacesState.context.getWorkspacesError}
      />
    </>
  )
}

export default WorkspacesPage
