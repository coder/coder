import { useActor } from "@xstate/react"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import dayjs from "dayjs"
import { useFeatureVisibility } from "hooks/useFeatureVisibility"
import { useEffect } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate } from "react-router-dom"
import {
  getDeadline,
  getMaxDeadline,
  getMaxDeadlineChange,
  getMinDeadline,
} from "util/schedule"
import { quotaMachine } from "xServices/quotas/quotasXService"
import { StateFrom } from "xstate"
import { DeleteDialog } from "../../components/Dialogs/DeleteDialog/DeleteDialog"
import {
  Workspace,
  WorkspaceErrors,
} from "../../components/Workspace/Workspace"
import { pageTitle } from "../../util/page"
import { getFaviconByStatus } from "../../util/workspace"
import {
  WorkspaceEvent,
  workspaceMachine,
} from "../../xServices/workspace/workspaceXService"

interface WorkspaceReadyPageProps {
  workspaceState: StateFrom<typeof workspaceMachine>
  quotaState: StateFrom<typeof quotaMachine>
  workspaceSend: (event: WorkspaceEvent) => void
}

export const WorkspaceReadyPage = ({
  workspaceState,
  quotaState,
  workspaceSend,
}: WorkspaceReadyPageProps): JSX.Element => {
  const [_, bannerSend] = useActor(
    workspaceState.children["scheduleBannerMachine"],
  )
  const { buildInfo } = useDashboard()
  const featureVisibility = useFeatureVisibility()
  const {
    workspace,
    template,
    templateParameters,
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
  const deadline = getDeadline(workspace)
  const canUpdateWorkspace = Boolean(permissions?.updateWorkspace)
  const { t } = useTranslation("workspacePage")
  const favicon = getFaviconByStatus(workspace.latest_build)
  const navigate = useNavigate()

  // keep banner machine in sync with workspace
  useEffect(() => {
    bannerSend({ type: "REFRESH_WORKSPACE", workspace })
  }, [bannerSend, workspace])

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${workspace.owner_name}/${workspace.name}`)}</title>
        <link
          rel="alternate icon"
          type="image/png"
          href={`/favicons/${favicon}.png`}
        />
        <link
          rel="icon"
          type="image/svg+xml"
          href={`/favicons/${favicon}.svg`}
        />
      </Helmet>

      <Workspace
        scheduleProps={{
          onDeadlineMinus: (hours: number) => {
            bannerSend({
              type: "DECREASE_DEADLINE",
              hours,
            })
          },
          onDeadlinePlus: (hours: number) => {
            bannerSend({
              type: "INCREASE_DEADLINE",
              hours,
            })
          },
          maxDeadlineDecrease: getMaxDeadlineChange(deadline, getMinDeadline()),
          maxDeadlineIncrease: getMaxDeadlineChange(
            getMaxDeadline(workspace),
            deadline,
          ),
        }}
        isUpdating={workspaceState.hasTag("updating")}
        workspace={workspace}
        handleStart={() => workspaceSend({ type: "START" })}
        handleStop={() => workspaceSend({ type: "STOP" })}
        handleDelete={() => workspaceSend({ type: "ASK_DELETE" })}
        handleUpdate={() => workspaceSend({ type: "UPDATE" })}
        handleCancel={() => workspaceSend({ type: "CANCEL" })}
        handleChangeVersion={() => navigate("change-version")}
        handleBuildParameters={() => navigate("build-parameters")}
        resources={workspace.latest_build.resources}
        builds={builds}
        canUpdateWorkspace={canUpdateWorkspace}
        hideSSHButton={featureVisibility["browser_only"]}
        hideVSCodeDesktopButton={featureVisibility["browser_only"]}
        workspaceErrors={{
          [WorkspaceErrors.GET_RESOURCES_ERROR]: refreshWorkspaceWarning,
          [WorkspaceErrors.GET_BUILDS_ERROR]: getBuildsError,
          [WorkspaceErrors.BUILD_ERROR]: buildError,
          [WorkspaceErrors.CANCELLATION_ERROR]: cancellationError,
        }}
        buildInfo={buildInfo}
        applicationsHost={applicationsHost}
        template={template}
        templateParameters={templateParameters}
        quota_budget={quotaState.context.quota?.budget}
      />
      <DeleteDialog
        entity="workspace"
        name={workspace.name}
        info={t("deleteDialog.info", {
          timeAgo: dayjs(workspace.created_at).fromNow(),
        })}
        isOpen={workspaceState.matches({ ready: { build: "askingDelete" } })}
        onCancel={() => workspaceSend({ type: "CANCEL_DELETE" })}
        onConfirm={() => {
          workspaceSend({ type: "DELETE" })
        }}
      />
    </>
  )
}
