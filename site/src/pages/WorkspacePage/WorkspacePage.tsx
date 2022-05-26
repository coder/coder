import { useMachine } from "@xstate/react"
import React, { useEffect } from "react"
import { useParams } from "react-router-dom"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { Workspace } from "../../components/Workspace/Workspace"
import { firstOrItem } from "../../util/array"
import { workspaceMachine } from "../../xServices/workspace/workspaceXService"

export const WorkspacePage: React.FC = () => {
  const { workspace: workspaceQueryParam } = useParams()
  const workspaceId = firstOrItem(workspaceQueryParam, null)

  const [workspaceState, workspaceSend] = useMachine(workspaceMachine)
  const { workspace, resources, getWorkspaceError, getResourcesError, builds } = workspaceState.context

  /**
   * Get workspace, template, and organization on mount and whenever workspaceId changes.
   * workspaceSend should not change.
   */
  useEffect(() => {
    workspaceId && workspaceSend({ type: "GET_WORKSPACE", workspaceId })
  }, [workspaceId, workspaceSend])

  if (workspaceState.matches("error")) {
    return <ErrorSummary error={getWorkspaceError} />
  } else if (!workspace) {
    return <FullScreenLoader />
  } else {
    return (
      <Margins>
        <Stack spacing={4}>
          <Workspace
            workspace={workspace}
            handleStart={() => workspaceSend("START")}
            handleStop={() => workspaceSend("STOP")}
            handleUpdate={() => workspaceSend("UPDATE")}
            handleCancel={() => workspaceSend("CANCEL")}
            resources={resources}
            getResourcesError={getResourcesError instanceof Error ? getResourcesError : undefined}
            builds={builds}
          />
        </Stack>
      </Margins>
    )
  }
}
