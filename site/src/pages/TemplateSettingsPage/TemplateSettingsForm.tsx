import TextField from "@material-ui/core/TextField"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { Stack } from "components/Stack/Stack"
import { FormikContextType, useFormik } from "formik"
import { FC } from "react"
import { getFormHelpersWithError, nameValidator, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"

export const Language = {
  nameLabel: "Name",
  descriptionLabel: "Description",
  maxTtlLabel: "Max TTL",
  // This is the same from the CLI on https://github.com/coder/coder/blob/546157b63ef9204658acf58cb653aa9936b70c49/cli/templateedit.go#L59
  maxTtlHelperText: "Edit the template maximum time before shutdown in milliseconds",
  formAriaLabel: "Template settings form",
}

export const validationSchema = Yup.object({
  name: nameValidator(Language.nameLabel),
  description: Yup.string(),
  max_ttl_ms: Yup.number(),
})

export interface TemplateSettingsForm {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  error?: unknown
}

export const TemplateSettingsForm: FC<TemplateSettingsForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
}) => {
  const form: FormikContextType<UpdateTemplateMeta> = useFormik<UpdateTemplateMeta>({
    initialValues: {
      name: template.name,
      description: template.description,
      max_ttl_ms: template.max_ttl_ms,
    },
    validationSchema,
    onSubmit: (data) => {
      form.setSubmitting(true)
      onSubmit(data)
    },
  })
  const getFieldHelpers = getFormHelpersWithError<UpdateTemplateMeta>(form, error)

  return (
    <form onSubmit={form.handleSubmit} aria-label={Language.formAriaLabel}>
      <Stack>
        <TextField
          {...getFieldHelpers("name")}
          disabled={form.isSubmitting}
          onChange={onChangeTrimmed(form)}
          autoFocus
          fullWidth
          label={Language.nameLabel}
          variant="outlined"
        />

        <TextField
          {...getFieldHelpers("description")}
          multiline
          disabled={form.isSubmitting}
          fullWidth
          label={Language.descriptionLabel}
          variant="outlined"
          rows={2}
        />

        <TextField
          {...getFieldHelpers("max_ttl_ms")}
          helperText={Language.maxTtlHelperText}
          disabled={form.isSubmitting}
          fullWidth
          inputProps={{ min: 0, step: 1 }}
          label={Language.maxTtlLabel}
          variant="outlined"
        />
      </Stack>

      <FormFooter onCancel={onCancel} isLoading={form.isSubmitting} />
    </form>
  )
}
