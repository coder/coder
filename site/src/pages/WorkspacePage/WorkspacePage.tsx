import { useMachine } from "@xstate/react"
import React, { useEffect } from "react"
import { Helmet } from "react-helmet"
import { useNavigate, useParams } from "react-router-dom"
import { DeleteWorkspaceDialog } from "../../components/DeleteWorkspaceDialog/DeleteWorkspaceDialog"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { Workspace } from "../../components/Workspace/Workspace"
import { firstOrItem } from "../../util/array"
import { pageTitle } from "../../util/page"
import { workspaceMachine } from "../../xServices/workspace/workspaceXService"
import { workspaceScheduleBannerMachine } from "../../xServices/workspaceSchedule/workspaceScheduleBannerXService"

export const WorkspacePage: React.FC = () => {
  const { workspace: workspaceQueryParam } = useParams()
  const navigate = useNavigate()
  const workspaceId = firstOrItem(workspaceQueryParam, null)

  const [workspaceState, workspaceSend] = useMachine(workspaceMachine)
  const { workspace, resources, getWorkspaceError, getResourcesError, builds } = workspaceState.context

  const [bannerState, bannerSend] = useMachine(workspaceScheduleBannerMachine)

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
        <Helmet>
          <title>{pageTitle(`${workspace.owner_name}/${workspace.name}`)}</title>
        </Helmet>
        <Stack spacing={4}>
          <>
            <Workspace
              bannerProps={{
                isLoading: bannerState.hasTag("loading"),
                onExtend: () => {
                  bannerSend({ type: "EXTEND_DEADLINE_DEFAULT", workspaceId: workspace.id })
                },
              }}
              workspace={workspace}
              handleStart={() => workspaceSend("START")}
              handleStop={() => workspaceSend("STOP")}
              handleDelete={() => workspaceSend("ASK_DELETE")}
              handleUpdate={() => workspaceSend("UPDATE")}
              handleCancel={() => workspaceSend("CANCEL")}
              resources={resources}
              getResourcesError={getResourcesError instanceof Error ? getResourcesError : undefined}
              builds={builds}
            />
            <DeleteWorkspaceDialog
              isOpen={workspaceState.matches({ ready: { build: "askingDelete" } })}
              handleCancel={() => workspaceSend("CANCEL_DELETE")}
              handleConfirm={() => {
                workspaceSend("DELETE")
                navigate("/workspaces")
              }}
            />
          </>
        </Stack>
      </Margins>
    )
  }
}
