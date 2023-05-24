import { useActor } from "@xstate/react"
import { ProvisionerJobLog } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import dayjs from "dayjs"
import { useFeatureVisibility } from "hooks/useFeatureVisibility"
import { FC, useEffect, useState } from "react"
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
import { useRestartWorkspace } from "./hooks"
import {
  ConfirmDialog,
  ConfirmDialogProps,
} from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { useMe } from "hooks/useMe"
import Checkbox from "@mui/material/Checkbox"
import FormControlLabel from "@mui/material/FormControlLabel"

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
    templateVersion,
    builds,
    getBuildsError,
    buildError,
    cancellationError,
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
  const [isConfirmingUpdate, setIsConfirmingUpdate] = useState(false)
  const [isConfirmingRestart, setIsConfirmingRestart] = useState(false)
  const user = useMe()
  const { isWarningIgnored, ignoreWarning } = useIgnoreWarnings(user.id)

  const {
    mutate: restartWorkspace,
    error: restartBuildError,
    isLoading: isRestarting,
  } = useRestartWorkspace()

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
        isRestarting={isRestarting}
        workspace={workspace}
        handleStart={() => workspaceSend({ type: "START" })}
        handleStop={() => workspaceSend({ type: "STOP" })}
        handleDelete={() => workspaceSend({ type: "ASK_DELETE" })}
        handleRestart={() => {
          if (isWarningIgnored("restart")) {
            restartWorkspace(workspace)
          } else {
            setIsConfirmingRestart(true)
          }
        }}
        handleUpdate={() => {
          if (isWarningIgnored("update")) {
            workspaceSend({ type: "UPDATE" })
          } else {
            setIsConfirmingUpdate(true)
          }
        }}
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
        canChangeVersions={canUpdateTemplate}
        hideSSHButton={featureVisibility["browser_only"]}
        hideVSCodeDesktopButton={featureVisibility["browser_only"]}
        workspaceErrors={{
          [WorkspaceErrors.GET_BUILDS_ERROR]: getBuildsError,
          [WorkspaceErrors.BUILD_ERROR]: buildError || restartBuildError,
          [WorkspaceErrors.CANCELLATION_ERROR]: cancellationError,
        }}
        buildInfo={buildInfo}
        sshPrefix={sshPrefix}
        template={template}
        quota_budget={quotaState.context.quota?.budget}
        templateWarnings={templateVersion?.warnings}
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
      <WarningDialog
        open={isConfirmingUpdate}
        onConfirm={(shouldIgnore) => {
          if (shouldIgnore) {
            ignoreWarning("update")
          }
          workspaceSend({ type: "UPDATE" })
          setIsConfirmingUpdate(false)
        }}
        onClose={() => setIsConfirmingUpdate(false)}
        title="Confirm update"
        confirmText="Update"
        description="Are you sure you want to update your workspace? Updating your workspace will stop all running processes and delete non-persistent data."
      />

      <WarningDialog
        open={isConfirmingRestart}
        onConfirm={(shouldIgnore) => {
          if (shouldIgnore) {
            ignoreWarning("restart")
          }
          restartWorkspace(workspace)
          setIsConfirmingRestart(false)
        }}
        onClose={() => setIsConfirmingRestart(false)}
        title="Confirm restart"
        confirmText="Restart"
        description="Are you sure you want to restart your workspace? Updating your workspace will stop all running processes and delete non-persistent data."
      />
    </>
  )
}

type IgnoredWarnings = Record<string, string>

const useIgnoreWarnings = (prefix: string) => {
  const ignoredWarningsJSON = localStorage.getItem(`${prefix}_ignoredWarnings`)
  let ignoredWarnings: IgnoredWarnings | undefined
  if (ignoredWarningsJSON) {
    ignoredWarnings = JSON.parse(ignoredWarningsJSON)
  }

  const isWarningIgnored = (warningId: string) => {
    return Boolean(ignoredWarnings?.[warningId])
  }

  const ignoreWarning = (warningId: string) => {
    if (!ignoredWarnings) {
      ignoredWarnings = {}
    }
    ignoredWarnings[warningId] = new Date().toISOString()
    localStorage.setItem(
      `${prefix}_ignoredWarnings`,
      JSON.stringify(ignoredWarnings),
    )
  }

  return {
    isWarningIgnored,
    ignoreWarning,
  }
}

const WarningDialog: FC<
  Pick<
    ConfirmDialogProps,
    "open" | "onClose" | "title" | "confirmText" | "description"
  > & { onConfirm: (shouldIgnore: boolean) => void }
> = ({ open, onConfirm, onClose, title, confirmText, description }) => {
  const [shouldIgnore, setShouldIgnore] = useState(false)

  return (
    <ConfirmDialog
      type="info"
      hideCancel={false}
      open={open}
      onConfirm={() => {
        onConfirm(shouldIgnore)
      }}
      onClose={onClose}
      title={title}
      confirmText={confirmText}
      description={
        <>
          <div>{description}</div>
          <FormControlLabel
            sx={{
              marginTop: 2,
            }}
            control={
              <Checkbox
                size="small"
                onChange={(e) => {
                  setShouldIgnore(e.target.checked)
                }}
              />
            }
            label="Don't show me this message again"
          />
        </>
      }
    />
  )
}
