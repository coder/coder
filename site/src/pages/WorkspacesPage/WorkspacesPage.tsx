import { useMachine , useActor } from "@xstate/react"
import React, { useContext } from "react"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"
import { XServiceContext } from "../../xServices/StateContext"

const WorkspacesPage: React.FC = () => {
  const xServices = useContext(XServiceContext)
  const [authState] = useActor(xServices.authXService)
  const { me } = authState.context

  const [workspacesState] = useMachine(workspacesMachine)
  

  return (
    <>
      <WorkspacesPageView
        loading={workspacesState.hasTag("loading")}
        workspaces={workspacesState.context.workspaces}
        me={me}
        error={workspacesState.context.getWorkspacesError}
      />
    </>
  )
}

export default WorkspacesPage
