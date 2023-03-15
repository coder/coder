import { getErrorMessage } from "api/errors"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { displayError } from "components/GlobalSnackbar/utils"
import { Loader } from "components/Loader/Loader"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { useUpdateWorkspaceSettings, useWorkspaceSettings } from "./data"
import { WorkspaceSettingsForm } from "./WorkspaceSettingsForm"

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

  const onCancel = () => navigate(-1)

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>

      <FullPageHorizontalForm title={t("title")} onCancel={onCancel}>
        <>
          {error && <AlertBanner error={error} severity="error" />}
          {isLoading && <Loader />}
          {settings && (
            <WorkspaceSettingsForm
              isSubmitting={updateSettings.isLoading}
              settings={settings}
              onCancel={onCancel}
              error={undefined}
              onSubmit={(formValues) => {
                updateSettings.mutate(formValues)
              }}
            />
          )}
        </>
      </FullPageHorizontalForm>
    </>
  )
}

export default WorkspaceSettingsPage
