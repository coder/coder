import { getErrorMessage } from "api/errors"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { displayError } from "components/GlobalSnackbar/utils"
import { Loader } from "components/Loader/Loader"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { useUpdateWorkspaceSettings, useWorkspaceSettings } from "./data"
import { WorkspaceSettingsForm } from "./WorkspaceSettingsForm"

const WorkspaceSettingsPage = () => {
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
      displayError(
        getErrorMessage(error, "Error on update workspace settings"),
      ),
  })

  const onCancel = () => navigate(-1)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Workspace Settings")}</title>
      </Helmet>

      <FullPageHorizontalForm title="Workspace settings" onCancel={onCancel}>
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
