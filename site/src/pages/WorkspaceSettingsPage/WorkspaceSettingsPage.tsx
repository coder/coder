import { getErrorMessage } from "api/errors"
import { displayError } from "components/GlobalSnackbar/utils"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { useUpdateWorkspaceSettings, useWorkspaceSettings } from "./data"
import { WorkspaceSettingsPageView } from "./WorkspaceSettingsPageView"

const WorkspaceSettingsPage = () => {
  const { t } = useTranslation("workspaceSettingsPage")
  const { username, workspace: workspaceName } = useParams() as {
    username: string
    workspace: string
  }
  const {
    data: settings,
    error,
    isLoading,
  } = useWorkspaceSettings(username, workspaceName)
  const navigate = useNavigate()
  const updateSettings = useUpdateWorkspaceSettings(settings?.workspace.id, {
    onSuccess: ({ name }) => {
      navigate(`/@${username}/${name}`)
    },
    onError: (error) =>
      displayError(getErrorMessage(error, t("defaultErrorMessage"))),
  })

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>

      <WorkspaceSettingsPageView
        formError={updateSettings.error}
        loadingError={error}
        isLoading={isLoading}
        isSubmitting={updateSettings.isLoading}
        settings={settings}
        onCancel={() => navigate(-1)}
        onSubmit={updateSettings.mutate}
      />
    </>
  )
}

export default WorkspaceSettingsPage
