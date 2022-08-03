import { useMachine, useSelector } from "@xstate/react"
import dayjs from "dayjs"
import minMax from "dayjs/plugin/minMax"
import React, { useContext, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { useParams } from "react-router-dom"
import { DeleteWorkspaceDialog } from "../../components/DeleteWorkspaceDialog/DeleteWorkspaceDialog"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Workspace } from "../../components/Workspace/Workspace"
import { firstOrItem } from "../../util/array"
import { pageTitle } from "../../util/page"
import { getFaviconByStatus } from "../../util/workspace"
import { selectUser } from "../../xServices/auth/authSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { workspaceMachine } from "../../xServices/workspace/workspaceXService"
import { workspaceScheduleBannerMachine } from "../../xServices/workspaceSchedule/workspaceScheduleBannerXService"

dayjs.extend(minMax)

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
    const favicon = getFaviconByStatus(workspace.latest_build)
    return (
      <>
        <Helmet>
          <title>{pageTitle(`${workspace.owner_name}/${workspace.name}`)}</title>
          <link rel="alternate icon" type="image/png" href={`/favicons/${favicon}.png`} />
          <link rel="icon" type="image/svg+xml" href={`/favicons/${favicon}.svg`} />
        </Helmet>

        <Workspace
          bannerProps={{
            isLoading: bannerState.hasTag("loading"),
            onExtend: () => {
              bannerSend({
                type: "UPDATE_DEADLINE",
                workspaceId: workspace.id,
                newDeadline: dayjs(workspace.latest_build.deadline).utc().add(4, "hours"),
              })
            },
          }}
          scheduleProps={{
            onDeadlineMinus: () => {
              bannerSend({
                type: "UPDATE_DEADLINE",
                workspaceId: workspace.id,
                newDeadline: boundedDeadline(
                  dayjs(workspace.latest_build.deadline).utc().add(-1, "hours"),
                  dayjs(),
                ),
              })
            },
            onDeadlinePlus: () => {
              bannerSend({
                type: "UPDATE_DEADLINE",
                workspaceId: workspace.id,
                newDeadline: boundedDeadline(
                  dayjs(workspace.latest_build.deadline).utc().add(1, "hours"),
                  dayjs(),
                ),
              })
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

export const boundedDeadline = (newDeadline: dayjs.Dayjs, now: dayjs.Dayjs): dayjs.Dayjs => {
  const minDeadline = now.add(30, "minutes")
  const maxDeadline = now.add(24, "hours")
  return dayjs.min(dayjs.max(minDeadline, newDeadline), maxDeadline)
}
