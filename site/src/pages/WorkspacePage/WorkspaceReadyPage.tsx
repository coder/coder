import { useActor } from "@xstate/react"
import { ProvisionerJobLog } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import dayjs from "dayjs"
import { useFeatureVisibility } from "hooks/useFeatureVisibility"
import { useEffect, useState } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate } from "react-router-dom"
import {
  getDeadline,
  getMaxDeadline,
  getMaxDeadlineChange,
  getMinDeadline,
} from "utils/schedule"
import { quotaMachine } from "xServices/quotas/quotasXService"
import { StateFrom } from "xstate"
import { DeleteDialog } from "../../components/Dialogs/DeleteDialog/DeleteDialog"
import {
  Workspace,
  WorkspaceErrors,
} from "../../components/Workspace/Workspace"
import { pageTitle } from "../../utils/page"
import { getFaviconByStatus } from "../../utils/workspace"
import {
  WorkspaceEvent,
  workspaceMachine,
} from "../../xServices/workspace/workspaceXService"
import { UpdateBuildParametersDialog } from "./UpdateBuildParametersDialog"
import { ChangeVersionDialog } from "./ChangeVersionDialog"
import { useQuery } from "@tanstack/react-query"
import { getTemplateVersions } from "api/api"

interface WorkspaceReadyPageProps {
  workspaceState: StateFrom<typeof workspaceMachine>
  quotaState: StateFrom<typeof quotaMachine>
  workspaceSend: (event: WorkspaceEvent) => void
  failedBuildLogs: ProvisionerJobLog[] | undefined
}

export const WorkspaceReadyPage = ({
  workspaceState,
  quotaState,
  failedBuildLogs,
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
    builds,
    getBuildsError,
    buildError,
    cancellationError,
    applicationsHost,
    sshPrefix,
    permissions,
    missedParameters,
  } = workspaceState.context
  if (workspace === undefined) {
    throw Error("Workspace is undefined")
  }
  const deadline = getDeadline(workspace)
  const canUpdateWorkspace = Boolean(permissions?.updateWorkspace)
  const canUpdateTemplate = Boolean(permissions?.updateTemplate)
  const { t } = useTranslation("workspacePage")
  const favicon = getFaviconByStatus(workspace.latest_build)
  const navigate = useNavigate()
  const [changeVersionDialogOpen, setChangeVersionDialogOpen] = useState(false)
  const { data: templateVersions } = useQuery({
    queryKey: ["template", "versions", workspace.template_id],
    queryFn: () => getTemplateVersions(workspace.template_id),
    enabled: changeVersionDialogOpen,
  })
  const dashboard = useDashboard()

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
        failedBuildLogs={failedBuildLogs}
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
        isUpdating={workspaceState.matches("ready.build.requestingUpdate")}
        workspace={workspace}
        handleStart={() => workspaceSend({ type: "START" })}
        handleStop={() => workspaceSend({ type: "STOP" })}
        handleRestart={() => workspaceSend({ type: "START" })}
        handleDelete={() => workspaceSend({ type: "ASK_DELETE" })}
        handleUpdate={() => workspaceSend({ type: "UPDATE" })}
        handleCancel={() => workspaceSend({ type: "CANCEL" })}
        handleSettings={() => navigate("settings")}
        handleBuildRetry={() => workspaceSend({ type: "RETRY_BUILD" })}
        handleChangeVersion={() => {
          setChangeVersionDialogOpen(true)
        }}
        resources={workspace.latest_build.resources}
        builds={builds}
        canUpdateWorkspace={canUpdateWorkspace}
        canUpdateTemplate={canUpdateTemplate}
        canChangeVersions={
          canUpdateTemplate && dashboard.experiments.includes("template_editor")
        }
        hideSSHButton={featureVisibility["browser_only"]}
        hideVSCodeDesktopButton={featureVisibility["browser_only"]}
        workspaceErrors={{
          [WorkspaceErrors.GET_BUILDS_ERROR]: getBuildsError,
          [WorkspaceErrors.BUILD_ERROR]: buildError,
          [WorkspaceErrors.CANCELLATION_ERROR]: cancellationError,
        }}
        buildInfo={buildInfo}
        applicationsHost={applicationsHost}
        sshPrefix={sshPrefix}
        template={template}
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
      <UpdateBuildParametersDialog
        missedParameters={missedParameters}
        open={workspaceState.matches(
          "ready.build.askingForMissedBuildParameters",
        )}
        onClose={() => {
          workspaceSend({ type: "CANCEL" })
        }}
        onUpdate={(buildParameters) => {
          workspaceSend({ type: "UPDATE", buildParameters })
        }}
      />
      <ChangeVersionDialog
        templateVersions={templateVersions?.reverse()}
        template={template}
        defaultTemplateVersion={templateVersions?.find(
          (v) => workspace.latest_build.template_version_id === v.id,
        )}
        open={changeVersionDialogOpen}
        onClose={() => {
          setChangeVersionDialogOpen(false)
        }}
        onConfirm={(templateVersion) => {
          setChangeVersionDialogOpen(false)
          workspaceSend({
            type: "CHANGE_VERSION",
            templateVersionId: templateVersion.id,
          })
        }}
      />
    </>
  )
}
