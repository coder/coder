import { makeStyles } from "@material-ui/core/styles"
import { useMachine } from "@xstate/react"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog"
import { Loader } from "components/Loader/Loader"
import { Margins } from "components/Margins/Margins"
import dayjs from "dayjs"
import { scheduleToAutoStart } from "pages/WorkspaceSchedulePage/schedule"
import { ttlMsToAutoStop } from "pages/WorkspaceSchedulePage/ttl"
import { useEffect, FC } from "react"
import { useTranslation } from "react-i18next"
import { Navigate, useNavigate, useParams } from "react-router-dom"
import { scheduleChanged } from "util/schedule"
import * as TypesGen from "../../api/typesGenerated"
import { WorkspaceScheduleForm } from "../../components/WorkspaceScheduleForm/WorkspaceScheduleForm"
import { firstOrItem } from "../../util/array"
import { workspaceSchedule } from "../../xServices/workspaceSchedule/workspaceScheduleXService"
import {
  formValuesToAutoStartRequest,
  formValuesToTTLRequest,
} from "./formToRequest"

const getAutoStart = (workspace?: TypesGen.Workspace) =>
  scheduleToAutoStart(workspace?.autostart_schedule)
const getAutoStop = (workspace?: TypesGen.Workspace) =>
  ttlMsToAutoStop(workspace?.ttl_ms)

const useStyles = makeStyles((theme) => ({
  topMargin: {
    marginTop: `${theme.spacing(3)}px`,
  },
}))

export const WorkspaceSchedulePage: FC = () => {
  const { t } = useTranslation("workspaceSchedulePage")
  const styles = useStyles()
  const { username: usernameQueryParam, workspace: workspaceQueryParam } =
    useParams()
  const navigate = useNavigate()
  const username = firstOrItem(usernameQueryParam, null)
  const workspaceName = firstOrItem(workspaceQueryParam, null)
  const [scheduleState, scheduleSend] = useMachine(workspaceSchedule)
  const {
    checkPermissionsError,
    submitScheduleError,
    getWorkspaceError,
    getTemplateError,
    permissions,
    workspace,
    template,
  } = scheduleState.context

  // Get workspace on mount and whenever the args for getting a workspace change.
  // scheduleSend should not change.
  useEffect(() => {
    username &&
      workspaceName &&
      scheduleSend({ type: "GET_WORKSPACE", username, workspaceName })
  }, [username, workspaceName, scheduleSend])

  if (!username || !workspaceName) {
    return <Navigate to="/workspaces" />
  }

  if (scheduleState.hasTag("loading") || !template) {
    return <Loader />
  }

  if (scheduleState.matches("error")) {
    return (
      <Margins>
        <div className={styles.topMargin}>
          <AlertBanner
            severity="error"
            error={
              getWorkspaceError || checkPermissionsError || getTemplateError
            }
            retry={() =>
              scheduleSend({ type: "GET_WORKSPACE", username, workspaceName })
            }
          />
        </div>
      </Margins>
    )
  }

  if (!permissions?.updateWorkspace) {
    return (
      <Margins>
        <div className={styles.topMargin}>
          <AlertBanner severity="error" error={Error(t("forbiddenError"))} />
        </div>
      </Margins>
    )
  }

  if (
    scheduleState.matches("presentForm") ||
    scheduleState.matches("submittingSchedule")
  ) {
    return (
      <WorkspaceScheduleForm
        submitScheduleError={submitScheduleError}
        initialValues={{
          ...getAutoStart(workspace),
          ...getAutoStop(workspace),
        }}
        isLoading={scheduleState.tags.has("loading")}
        defaultTTL={dayjs.duration(template.default_ttl_ms, "ms").asHours()}
        onCancel={() => {
          navigate(`/@${username}/${workspaceName}`)
        }}
        onSubmit={(values) => {
          scheduleSend({
            type: "SUBMIT_SCHEDULE",
            autoStart: formValuesToAutoStartRequest(values),
            ttl: formValuesToTTLRequest(values),
            autoStartChanged: scheduleChanged(getAutoStart(workspace), values),
            autoStopChanged: scheduleChanged(getAutoStop(workspace), values),
          })
        }}
      />
    )
  }

  if (scheduleState.matches("showingRestartDialog")) {
    return (
      <ConfirmDialog
        open
        title={t("dialogTitle")}
        description={t("dialogDescription")}
        confirmText={t("restart")}
        cancelText={t("applyLater")}
        hideCancel={false}
        onConfirm={() => {
          scheduleSend("RESTART_WORKSPACE")
        }}
        onClose={() => {
          scheduleSend("APPLY_LATER")
        }}
      />
    )
  }

  if (scheduleState.matches("done")) {
    return <Navigate to={`/@${username}/${workspaceName}`} />
  }

  // Theoretically impossible - log and bail
  console.error("WorkspaceSchedulePage: unknown state :: ", scheduleState)
  return <Navigate to="/" />
}

export default WorkspaceSchedulePage
