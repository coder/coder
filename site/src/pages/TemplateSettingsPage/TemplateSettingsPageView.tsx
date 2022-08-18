import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Loader } from "components/Loader/Loader"
import { FC } from "react"
import { TemplateSettingsForm } from "./TemplateSettingsForm"

export const Language = {
  title: "Template settings",
}

export interface TemplateSettingsPageViewProps {
  template?: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  errors?: {
    getTemplateError?: unknown
    saveTemplateSettingsError?: unknown
  }
}

export const TemplateSettingsPageView: FC<TemplateSettingsPageViewProps> = ({
  template,
  onCancel,
  onSubmit,
  errors = {},
}) => {
  const isLoading = !template && !errors.getTemplateError

  return (
    <FullPageForm title={Language.title} onCancel={onCancel}>
      {errors.getTemplateError && <ErrorSummary error={errors.getTemplateError} />}
      {isLoading && <Loader />}
      {template && (
        <TemplateSettingsForm
          template={template}
          onSubmit={onSubmit}
          onCancel={onCancel}
          error={errors.saveTemplateSettingsError}
        />
      )}
    </FullPageForm>
  )
}
