import { makeStyles } from "@material-ui/core/styles"
import { useActor, useMachine, useSelector } from "@xstate/react"
import { FeatureNames } from "api/types"
import dayjs from "dayjs"
import { FC, useContext, useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useParams } from "react-router-dom"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { StateFrom } from "xstate"
import { DeleteDialog } from "../../components/Dialogs/DeleteDialog/DeleteDialog"
import { ErrorSummary } from "../../components/ErrorSummary/ErrorSummary"
import { FullScreenLoader } from "../../components/Loader/FullScreenLoader"
import { Workspace, WorkspaceErrors } from "../../components/Workspace/Workspace"
import { firstOrItem } from "../../util/array"
import { pageTitle } from "../../util/page"
import { getFaviconByStatus } from "../../util/workspace"
import { XServiceContext } from "../../xServices/StateContext"
import {
  WorkspaceEvent,
  workspaceMachine,
} from "../../xServices/workspace/workspaceXService"

export const WorkspacePage: FC = () => {
  const { username: usernameQueryParam, workspace: workspaceQueryParam } = useParams()
  const username = firstOrItem(usernameQueryParam, null)
  const workspaceName = firstOrItem(workspaceQueryParam, null)
  const [workspaceState, workspaceSend] = useMachine(workspaceMachine)
  const { workspace, getWorkspaceError, getTemplateWarning, checkPermissionsError } =
    workspaceState.context
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
        {Boolean(getTemplateWarning) && <ErrorSummary error={getTemplateWarning} />}
        {Boolean(checkPermissionsError) && <ErrorSummary error={checkPermissionsError} />}
      </div>
    )
  } else if (workspace && workspaceState.matches("ready")) {
    return <WorkspaceReadyPage workspaceState={workspaceState} workspaceSend={workspaceSend} />
  } else {
    return <FullScreenLoader />
  }
}

interface WorkspaceReadyPageProps {
  workspaceState: StateFrom<typeof workspaceMachine>
  workspaceSend: (event: WorkspaceEvent) => void
}

export const WorkspaceReadyPage = ({
  workspaceState,
  workspaceSend,
}: WorkspaceReadyPageProps): JSX.Element => {
  const [bannerState, bannerSend] = useActor(workspaceState.children["scheduleBannerMachine"])
  const xServices = useContext(XServiceContext)
  const featureVisibility = useSelector(xServices.entitlementsXService, selectFeatureVisibility)
  const [buildInfoState] = useActor(xServices.buildInfoXService)
  const {
    workspace,
    refreshWorkspaceWarning,
    builds,
    getBuildsError,
    buildError,
    cancellationError,
    applicationsHost,
    permissions,
  } = workspaceState.context
  if (workspace === undefined) {
    throw Error("Workspace is undefined")
  }
  const canUpdateWorkspace = Boolean(permissions?.updateWorkspace)
  const { t } = useTranslation("workspacePage")
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
              type: "INCREASE_DEADLINE",
              hours: 4,
            })
          },
        }}
        scheduleProps={{
          onDeadlineMinus: () => {
            bannerSend({
              type: "DECREASE_DEADLINE",
              hours: 1,
            })
          },
          onDeadlinePlus: () => {
            bannerSend({
              type: "INCREASE_DEADLINE",
              hours: 1,
            })
          },
          deadlineMinusEnabled: () => {
            return !bannerState.matches("atMinDeadline")
          },
          deadlinePlusEnabled: () => {
            return !bannerState.matches("atMaxDeadline")
          },
        }}
        isUpdating={workspaceState.hasTag("updating")}
        workspace={workspace}
        handleStart={() => workspaceSend({ type: "START" })}
        handleStop={() => workspaceSend({ type: "STOP" })}
        handleDelete={() => workspaceSend({ type: "ASK_DELETE" })}
        handleUpdate={() => workspaceSend({ type: "UPDATE" })}
        handleCancel={() => workspaceSend({ type: "CANCEL" })}
        resources={workspace.latest_build.resources}
        builds={builds}
        canUpdateWorkspace={canUpdateWorkspace}
        hideSSHButton={featureVisibility[FeatureNames.BrowserOnly]}
        workspaceErrors={{
          [WorkspaceErrors.GET_RESOURCES_ERROR]: refreshWorkspaceWarning,
          [WorkspaceErrors.GET_BUILDS_ERROR]: getBuildsError,
          [WorkspaceErrors.BUILD_ERROR]: buildError,
          [WorkspaceErrors.CANCELLATION_ERROR]: cancellationError,
        }}
        buildInfo={buildInfoState.context.buildInfo}
        applicationsHost={applicationsHost}
      />
      <DeleteDialog
        entity="workspace"
        name={workspace.name}
        info={t("deleteDialog.info", { timeAgo: dayjs(workspace.created_at).fromNow() })}
        isOpen={workspaceState.matches({ ready: { build: "askingDelete" } })}
        onCancel={() => workspaceSend({ type: "CANCEL_DELETE" })}
        onConfirm={() => {
          workspaceSend({ type: "DELETE" })
        }}
      />
    </>
  )
}

const useStyles = makeStyles((theme) => ({
  error: {
    margin: theme.spacing(2),
  },
}))

export default WorkspacePage
