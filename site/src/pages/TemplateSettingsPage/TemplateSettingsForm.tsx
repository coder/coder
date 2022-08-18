import TextField from "@material-ui/core/TextField"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { Stack } from "components/Stack/Stack"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC } from "react"
import { getFormHelpersWithError, nameValidator, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"

export const Language = {
  nameLabel: "Name",
  descriptionLabel: "Description",
  maxTtlLabel: "Auto-stop limit",
  // This is the same from the CLI on https://github.com/coder/coder/blob/546157b63ef9204658acf58cb653aa9936b70c49/cli/templateedit.go#L59
  maxTtlHelperText: "Edit the template maximum time before shutdown in seconds",
  formAriaLabel: "Template settings form",
}

export const validationSchema = Yup.object({
  name: nameValidator(Language.nameLabel),
  description: Yup.string(),
  max_ttl_ms: Yup.number(),
})
export interface UpdateTemplateFormMeta {
  readonly name?: string
  readonly description?: string
  readonly max_ttl?: number
}

export interface TemplateSettingsForm {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  error?: unknown
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateFormMeta>
}

export const TemplateSettingsForm: FC<TemplateSettingsForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
  isSubmitting,
  initialTouched,
}) => {
  const form: FormikContextType<UpdateTemplateFormMeta> = useFormik<UpdateTemplateFormMeta>({
    initialValues: {
      name: template.name,
      description: template.description,
      max_ttl: template.max_ttl_ms / 1000,
    },
    validationSchema,
    onSubmit: (data) => {
      onSubmit({
        ...data,
        max_ttl_ms: data.max_ttl !== undefined ? data.max_ttl * 1000 : undefined,
      })
    },
    initialTouched,
  })
  const getFieldHelpers = getFormHelpersWithError<UpdateTemplateFormMeta>(form, error)

  return (
    <form onSubmit={form.handleSubmit} aria-label={Language.formAriaLabel}>
      <Stack>
        <TextField
          {...getFieldHelpers("name")}
          disabled={isSubmitting}
          onChange={onChangeTrimmed(form)}
          autoFocus
          fullWidth
          label={Language.nameLabel}
          variant="outlined"
        />

        <TextField
          {...getFieldHelpers("description")}
          multiline
          disabled={isSubmitting}
          fullWidth
          label={Language.descriptionLabel}
          variant="outlined"
          rows={2}
        />

        <TextField
          {...getFieldHelpers("max_ttl")}
          helperText={Language.maxTtlHelperText}
          disabled={isSubmitting}
          fullWidth
          inputProps={{ min: 0, step: 1 }}
          label={Language.maxTtlLabel}
          variant="outlined"
        />
      </Stack>

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </form>
  )
}
