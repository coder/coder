import { useMachine } from "@xstate/react"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesPageView } from "./WorkspacesPageView"

dayjs.extend(relativeTime)

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
