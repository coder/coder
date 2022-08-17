import { makeStyles } from "@material-ui/core/styles"
import { useMachine, useSelector } from "@xstate/react"
import dayjs from "dayjs"
import minMax from "dayjs/plugin/minMax"
import React, { useContext, useEffect } from "react"
import { Helmet } from "react-helmet"
import { useParams } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { DeleteWorkspaceDialog } from "../../components/DeleteWorkspaceDialog/DeleteWorkspaceDialog"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Workspace, WorkspaceErrors } from "../../components/Workspace/Workspace"
import { firstOrItem } from "../../util/array"
import { pageTitle } from "../../util/page"
import { getFaviconByStatus } from "../../util/workspace"
import { selectUser } from "../../xServices/auth/authSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { workspaceMachine } from "../../xServices/workspace/workspaceXService"
import { workspaceScheduleBannerMachine } from "../../xServices/workspaceSchedule/workspaceScheduleBannerXService"

dayjs.extend(minMax)

export const deadlineExtensionMin = dayjs.duration(30, "minutes")
export const deadlineExtensionMax = dayjs.duration(24, "hours")

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
  const {
    workspace,
    template,
    getWorkspaceError,
    resources,
    getResourcesError,
    builds,
    getBuildsError,
    permissions,
    checkPermissionsError,
    buildError,
    cancellationError,
  } = workspaceState.context

  const canUpdateWorkspace = !!permissions?.updateWorkspace

  const [bannerState, bannerSend] = useMachine(workspaceScheduleBannerMachine)

  const styles = useStyles()

  /**
   * Get workspace, template, and organization on mount and whenever workspaceId changes.
   * workspaceSend should not change.
   */
  useEffect(() => {
    username && workspaceName && workspaceSend({ type: "GET_WORKSPACE", username, workspaceName })
  }, [username, workspaceName, workspaceSend])

  if (workspaceState.matches("error")) {
    return (
      <div className={styles.error}>
        {getWorkspaceError && <ErrorSummary error={getWorkspaceError} />}
        {checkPermissionsError && <ErrorSummary error={checkPermissionsError} />}
      </div>
    )
  } else if (!workspace || !template) {
    return <FullScreenLoader />
  } else {
    const now = dayjs().utc()
    const deadline = dayjs(workspace.latest_build.deadline).utc()
    const dmax = maxDeadline(workspace, template)
    const dmin = minDeadline(now)
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
                newDeadline: dayjs.min(deadline.add(4, "hours"), dmax),
              })
            },
          }}
          scheduleProps={{
            onDeadlineMinus: () => {
              bannerSend({
                type: "UPDATE_DEADLINE",
                workspaceId: workspace.id,
                newDeadline: dayjs.max(deadline.add(-1, "hours"), dmin),
              })
            },
            onDeadlinePlus: () => {
              bannerSend({
                type: "UPDATE_DEADLINE",
                workspaceId: workspace.id,
                newDeadline: dayjs.min(deadline.add(1, "hours"), dmin),
              })
            },
            deadlineMinusEnabled: () => {
              return deadline > dmin
            },
            deadlinePlusEnabled: () => {
              return deadline < dmax
            },
          }}
          workspace={workspace}
          handleStart={() => workspaceSend("START")}
          handleStop={() => workspaceSend("STOP")}
          handleDelete={() => workspaceSend("ASK_DELETE")}
          handleUpdate={() => workspaceSend("UPDATE")}
          handleCancel={() => workspaceSend("CANCEL")}
          resources={resources}
          builds={builds}
          canUpdateWorkspace={canUpdateWorkspace}
          workspaceErrors={{
            [WorkspaceErrors.GET_RESOURCES_ERROR]: getResourcesError,
            [WorkspaceErrors.GET_BUILDS_ERROR]: getBuildsError,
            [WorkspaceErrors.BUILD_ERROR]: buildError,
            [WorkspaceErrors.CANCELLATION_ERROR]: cancellationError,
          }}
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

export function maxDeadline(ws: TypesGen.Workspace, tpl: TypesGen.Template): dayjs.Dayjs {
  const startedAt = dayjs(ws.latest_build.updated_at)
  const templateMaxTTL = dayjs.duration(tpl.max_ttl_ms, "milliseconds")
  const maxTemplateDeadline = startedAt.add(templateMaxTTL)
  const maxGlobalDeadline = startedAt.add(deadlineExtensionMax)
  return dayjs.min(maxTemplateDeadline, maxGlobalDeadline)
}

export function minDeadline(now: dayjs.Dayjs): dayjs.Dayjs {
  return now.add(deadlineExtensionMin)
}

const useStyles = makeStyles((theme) => ({
  error: {
    margin: theme.spacing(2),
  },
}))
