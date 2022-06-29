import { useMachine, useSelector } from "@xstate/react"
import React, { useContext, useEffect } from "react"
import { Helmet } from "react-helmet"
import { useParams } from "react-router-dom"
import { DeleteWorkspaceDialog } from "../../components/DeleteWorkspaceDialog/DeleteWorkspaceDialog"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Workspace } from "../../components/Workspace/Workspace"
import { firstOrItem } from "../../util/array"
import { pageTitle } from "../../util/page"
import { selectUser } from "../../xServices/auth/authSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { workspaceMachine } from "../../xServices/workspace/workspaceXService"
import { workspaceScheduleBannerMachine } from "../../xServices/workspaceSchedule/workspaceScheduleBannerXService"

export const WorkspacePage: React.FC = () => {
  const { username: usernameQueryParam, workspace: workspaceQueryParam } = useParams()
  const username = firstOrItem(usernameQueryParam, null)
  const workspaceName = firstOrItem(workspaceQueryParam, null)

  const xServices = useContext(XServiceContext)
  const me = useSelector(xServices.authXService, selectUser)

  const [workspaceState, workspaceSend] = useMachine(workspaceMachine, {
    context: {
      userId: me?.id,
    },
  })
  const { workspace, resources, getWorkspaceError, getResourcesError, builds, permissions } =
    workspaceState.context

  const canUpdateWorkspace = !!permissions?.updateWorkspace

  const [bannerState, bannerSend] = useMachine(workspaceScheduleBannerMachine)

  /**
   * Get workspace, template, and organization on mount and whenever workspaceId changes.
   * workspaceSend should not change.
   */
  useEffect(() => {
    username && workspaceName && workspaceSend({ type: "GET_WORKSPACE", username, workspaceName })
  }, [username, workspaceName, workspaceSend])

  if (workspaceState.matches("error")) {
    return <ErrorSummary error={getWorkspaceError} />
  } else if (!workspace) {
    return <FullScreenLoader />
  } else {
    return (
      <>
        <Helmet>
          <title>{pageTitle(`${workspace.owner_name}/${workspace.name}`)}</title>
        </Helmet>

        <Workspace
          bannerProps={{
            isLoading: bannerState.hasTag("loading"),
            onExtend: () => {
              bannerSend({ type: "EXTEND_DEADLINE_DEFAULT", workspaceId: workspace.id })
            },
          }}
          scheduleProps={{
            onDeadlineMinus: () => {
              console.log("not implemented")
            },
            onDeadlinePlus: () => {
              console.log("not implemented")
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
          canUpdateWorkspace={canUpdateWorkspace}
        />
        <DeleteWorkspaceDialog
          isOpen={workspaceState.matches({ ready: { build: "askingDelete" } })}
          handleCancel={() => workspaceSend("CANCEL_DELETE")}
          handleConfirm={() => {
            workspaceSend("DELETE")
          }}
        />
      </>
    )
  }
}
