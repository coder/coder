import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { AlertBanner } from "components/AlertBanner/AlertBanner"
import { FullPageForm } from "components/FullPageForm/FullPageForm"
import { Loader } from "components/Loader/Loader"
import { ComponentProps, FC } from "react"
import { TemplateSettingsForm } from "./TemplateSettingsForm"
import { Stack } from "components/Stack/Stack"
import { DeleteButton } from "components/DropdownButton/ActionCtas"
import { DeleteDialog } from "components/Dialogs/DeleteDialog/DeleteDialog"

export const Language = {
  title: "Template settings",
}

export interface TemplateSettingsPageViewProps {
  template?: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  onDelete: () => void
  onConfirmDelete: () => void
  onCancelDelete: () => void
  isConfirmingDelete: boolean
  isDeleting: boolean
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
  onDelete,
  onConfirmDelete,
  onCancelDelete,
  isConfirmingDelete,
  isDeleting,
  isSubmitting,
  errors = {},
  initialTouched,
}) => {
  const isLoading = !template && !errors.getTemplateError

  return (
    <FullPageForm title={Language.title} onCancel={onCancel}>
      {Boolean(errors.getTemplateError) && (
        <AlertBanner severity="error" error={errors.getTemplateError} />
      )}
      {isLoading && <Loader />}
      {template && (
        <>
          <TemplateSettingsForm
            initialTouched={initialTouched}
            isSubmitting={isSubmitting}
            template={template}
            onSubmit={onSubmit}
            onCancel={onCancel}
            error={errors.saveTemplateSettingsError}
          />
          <Stack>
            Danger Zone
            <DeleteButton handleAction={onDelete} />
          </Stack>
          <DeleteDialog
            isOpen={isConfirmingDelete}
            confirmLoading={isDeleting}
            onConfirm={onConfirmDelete}
            onCancel={onCancelDelete}
            entity="template"
            name={template.name}
          />
        </>
      )}
    </FullPageForm>
  )
}
