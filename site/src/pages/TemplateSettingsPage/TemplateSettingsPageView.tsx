import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Loader } from "components/Loader/Loader"
import { ComponentProps, FC } from "react"
import { TemplateSettingsForm } from "./TemplateSettingsForm"

export const Language = {
  title: "Template settings",
}

export interface TemplateSettingsPageViewProps {
  template?: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  errors?: {
    getTemplateError?: unknown
    saveTemplateSettingsError?: unknown
  }
  initialTouched?: ComponentProps<typeof TemplateSettingsForm>["initialTouched"]
}

export const TemplateSettingsPageView: FC<TemplateSettingsPageViewProps> = ({
  template,
  onCancel,
  onSubmit,
  isSubmitting,
  errors = {},
  initialTouched,
}) => {
  const isLoading = !template && !errors.getTemplateError

  return (
    <FullPageForm title={Language.title} onCancel={onCancel}>
      {errors.getTemplateError && <ErrorSummary error={errors.getTemplateError} />}
      {isLoading && <Loader />}
      {template && (
        <TemplateSettingsForm
          initialTouched={initialTouched}
          isSubmitting={isSubmitting}
          template={template}
          onSubmit={onSubmit}
          onCancel={onCancel}
          error={errors.saveTemplateSettingsError}
        />
      )}
    </FullPageForm>
  )
}
