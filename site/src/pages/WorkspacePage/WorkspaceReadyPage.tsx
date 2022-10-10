import { useActor, useSelector } from "@xstate/react"
import { FeatureNames } from "api/types"
import dayjs from "dayjs"
import { useContext } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { selectFeatureVisibility } from "xServices/entitlements/entitlementsSelectors"
import { StateFrom } from "xstate"
import { DeleteDialog } from "../../components/Dialogs/DeleteDialog/DeleteDialog"
import { Workspace, WorkspaceErrors } from "../../components/Workspace/Workspace"
import { pageTitle } from "../../util/page"
import { getFaviconByStatus } from "../../util/workspace"
import { XServiceContext } from "../../xServices/StateContext"
import { WorkspaceEvent, workspaceMachine } from "../../xServices/workspace/workspaceXService"

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
          deadlineMinusEnabled: () => !bannerState.matches("atMinDeadline"),
          deadlinePlusEnabled: () => !bannerState.matches("atMaxDeadline"),
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
