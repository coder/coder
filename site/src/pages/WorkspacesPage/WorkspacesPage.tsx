import { useMachine } from "@xstate/react"
import React from "react"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

export const WorkspacesPage: React.FC = () => {
  const [workspacesState] = useMachine(workspacesMachine)

  return (
    <>
      <WorkspacesPageView
        workspaces={workspacesState.context.workspaces}
        error={workspacesState.context.getWorkspacesError}
      />
    </>
  )
}
