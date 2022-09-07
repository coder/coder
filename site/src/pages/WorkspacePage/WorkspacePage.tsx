import { makeStyles } from "@material-ui/core/styles"
import { useActor, useMachine, useSelector } from "@xstate/react"
import dayjs from "dayjs"
import minMax from "dayjs/plugin/minMax"
import { FC, useContext, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useParams } from "react-router-dom"
import { DeleteDialog } from "../../components/Dialogs/DeleteDialog/DeleteDialog"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Workspace, WorkspaceErrors } from "../../components/Workspace/Workspace"
import { firstOrItem } from "../../util/array"
import { pageTitle } from "../../util/page"
import { canExtendDeadline, canReduceDeadline, maxDeadline, minDeadline } from "../../util/schedule"
import { getFaviconByStatus } from "../../util/workspace"
import { selectUser } from "../../xServices/auth/authSelectors"
import { XServiceContext } from "../../xServices/StateContext"
import { workspaceMachine } from "../../xServices/workspace/workspaceXService"
import { workspaceScheduleBannerMachine } from "../../xServices/workspaceSchedule/workspaceScheduleBannerXService"

dayjs.extend(minMax)

export const WorkspacePage: FC = () => {
  const { username: usernameQueryParam, workspace: workspaceQueryParam } = useParams()
  const username = firstOrItem(usernameQueryParam, null)
  const workspaceName = firstOrItem(workspaceQueryParam, null)

  const { t } = useTranslation("workspacePage")

  const xServices = useContext(XServiceContext)
  const me = useSelector(xServices.authXService, selectUser)

  const [workspaceState, workspaceSend] = useMachine(workspaceMachine, {
    context: {
      userId: me?.id,
    },
  })
  const {
    workspace,
    getWorkspaceError,
    template,
    refreshTemplateError,
    resources,
    getResourcesError,
    builds,
    getBuildsError,
    permissions,
    checkPermissionsError,
    buildError,
    cancellationError,
  } = workspaceState.context

  const canUpdateWorkspace = Boolean(permissions?.updateWorkspace)

  const [bannerState, bannerSend] = useMachine(workspaceScheduleBannerMachine)
  const [buildInfoState] = useActor(xServices.buildInfoXService)

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
        {Boolean(getWorkspaceError) && <ErrorSummary error={getWorkspaceError} />}
        {Boolean(refreshTemplateError) && <ErrorSummary error={refreshTemplateError} />}
        {Boolean(checkPermissionsError) && <ErrorSummary error={checkPermissionsError} />}
      </div>
    )
  } else if (!workspace || !permissions) {
    return <FullScreenLoader />
  } else if (!template) {
    return <FullScreenLoader />
  } else {
    const deadline = dayjs(workspace.latest_build.deadline).utc()
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
                newDeadline: dayjs.min(deadline.add(4, "hours"), maxDeadline(workspace, template)),
              })
            },
          }}
          scheduleProps={{
            onDeadlineMinus: () => {
              bannerSend({
                type: "UPDATE_DEADLINE",
                workspaceId: workspace.id,
                newDeadline: dayjs.max(deadline.add(-1, "hours"), minDeadline()),
              })
            },
            onDeadlinePlus: () => {
              bannerSend({
                type: "UPDATE_DEADLINE",
                workspaceId: workspace.id,
                newDeadline: dayjs.min(deadline.add(1, "hours"), maxDeadline(workspace, template)),
              })
            },
            deadlineMinusEnabled: () => {
              return canReduceDeadline(deadline)
            },
            deadlinePlusEnabled: () => {
              return canExtendDeadline(deadline, workspace, template)
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
          buildInfo={buildInfoState.context.buildInfo}
        />
        <DeleteDialog
          title={t("deleteDialog.title")}
          description={t("deleteDialog.description")}
          isOpen={workspaceState.matches({ ready: { build: "askingDelete" } })}
          onCancel={() => workspaceSend("CANCEL_DELETE")}
          onConfirm={() => {
            workspaceSend("DELETE")
          }}
        />
      </>
    )
  }
}

const useStyles = makeStyles((theme) => ({
  error: {
    margin: theme.spacing(2),
  },
}))
