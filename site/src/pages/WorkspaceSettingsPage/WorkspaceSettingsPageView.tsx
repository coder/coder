import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { FullPageHorizontalForm } from "components/FullPageForm/FullPageHorizontalForm"
import { Loader } from "components/Loader/Loader"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { WorkspaceSettings, WorkspaceSettingsFormValue } from "./data"
import { WorkspaceSettingsForm } from "./WorkspaceSettingsForm"

export type WorkspaceSettingsPageViewProps = {
  formError: unknown
  loadingError: unknown
  isLoading: boolean
  isSubmitting: boolean
  settings: WorkspaceSettings | undefined
  onCancel: () => void
  onSubmit: (formValues: WorkspaceSettingsFormValue) => void
}

export const WorkspaceSettingsPageView: FC<WorkspaceSettingsPageViewProps> = ({
  onCancel,
  onSubmit,
  isLoading,
  isSubmitting,
  settings,
  formError,
  loadingError,
}) => {
  const { t } = useTranslation("workspaceSettingsPage")

  return (
    <FullPageHorizontalForm title={t("title")} onCancel={onCancel}>
      <>
        {loadingError && <AlertBanner error={loadingError} severity="error" />}
        {isLoading && <Loader />}
        {settings && (
          <WorkspaceSettingsForm
            error={formError}
            isSubmitting={isSubmitting}
            settings={settings}
            onCancel={onCancel}
            onSubmit={onSubmit}
          />
        )}
      </>
    </FullPageHorizontalForm>
  )
}
