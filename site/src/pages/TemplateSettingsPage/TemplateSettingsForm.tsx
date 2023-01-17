import Box from "@material-ui/core/Box"
import Checkbox from "@material-ui/core/Checkbox"
import Typography from "@material-ui/core/Typography"
import TextField from "@material-ui/core/TextField"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { Stack } from "components/Stack/Stack"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC } from "react"
import {
  getFormHelpers,
  nameValidator,
  templateDisplayNameValidator,
  onChangeTrimmed,
} from "util/formUtils"
import * as Yup from "yup"
import i18next from "i18next"
import { useTranslation } from "react-i18next"
import { Maybe } from "components/Conditionals/Maybe"
import { LazyIconField } from "components/IconField/LazyIconField"

const TTLHelperText = ({ ttl }: { ttl?: number }) => {
  const { t } = useTranslation("templateSettingsPage")
  const count = typeof ttl !== "number" ? 0 : ttl
  return (
    // no helper text if ttl is negative - error will show once field is considered touched
    <Maybe condition={count >= 0}>
      <span>{t("ttlHelperText", { count })}</span>
    </Maybe>
  )
}

const MAX_DESCRIPTION_CHAR_LIMIT = 128
const MAX_TTL_DAYS = 7
const MS_HOUR_CONVERSION = 3600000

export const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
    name: nameValidator(i18next.t("nameLabel", { ns: "templateSettingsPage" })),
    display_name: templateDisplayNameValidator(
      i18next.t("displayNameLabel", {
        ns: "templateSettingsPage",
      }),
    ),
    description: Yup.string().max(
      MAX_DESCRIPTION_CHAR_LIMIT,
      i18next.t("descriptionMaxError", { ns: "templateSettingsPage" }),
    ),
    default_ttl_ms: Yup.number()
      .integer()
      .min(0, i18next.t("ttlMinError", { ns: "templateSettingsPage" }))
      .max(
        24 * MAX_TTL_DAYS /* 7 days in hours */,
        i18next.t("ttlMaxError", { ns: "templateSettingsPage" }),
      ),
    allow_user_cancel_workspace_jobs: Yup.boolean(),
  })

export interface TemplateSettingsForm {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  error?: unknown
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateMeta>
}

export const TemplateSettingsForm: FC<TemplateSettingsForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
  isSubmitting,
  initialTouched,
}) => {
  const validationSchema = getValidationSchema()
  const form: FormikContextType<UpdateTemplateMeta> =
    useFormik<UpdateTemplateMeta>({
      initialValues: {
        name: template.name,
        display_name: template.display_name,
        description: template.description,
        // on display, convert from ms => hours
        default_ttl_ms: template.default_ttl_ms / MS_HOUR_CONVERSION,
        icon: template.icon,
        allow_user_cancel_workspace_jobs:
          template.allow_user_cancel_workspace_jobs,
      },
      validationSchema,
      onSubmit: (formData) => {
        // on submit, convert from hours => ms
        onSubmit({
          ...formData,
          default_ttl_ms: formData.default_ttl_ms
            ? formData.default_ttl_ms * MS_HOUR_CONVERSION
            : undefined,
        })
      },
      initialTouched,
    })
  const getFieldHelpers = getFormHelpers<UpdateTemplateMeta>(form, error)
  const { t } = useTranslation("templateSettingsPage")

  return (
    <form onSubmit={form.handleSubmit} aria-label={t("formAriaLabel")}>
      <Stack>
        <TextField
          {...getFieldHelpers("name")}
          disabled={isSubmitting}
          onChange={onChangeTrimmed(form)}
          autoFocus
          fullWidth
          label={t("nameLabel")}
          variant="outlined"
        />

        <TextField
          {...getFieldHelpers("display_name")}
          disabled={isSubmitting}
          fullWidth
          label={t("displayNameLabel")}
          variant="outlined"
        />

        <TextField
          {...getFieldHelpers("description")}
          multiline
          disabled={isSubmitting}
          fullWidth
          label={t("descriptionLabel")}
          variant="outlined"
          rows={2}
        />

        <LazyIconField
          {...getFieldHelpers("icon")}
          disabled={isSubmitting}
          onChange={onChangeTrimmed(form)}
          fullWidth
          label={t("form.fields.icon")}
          variant="outlined"
          onPickEmoji={(value) => form.setFieldValue("icon", value)}
        />

        <TextField
          {...getFieldHelpers(
            "default_ttl_ms",
            <TTLHelperText ttl={form.values.default_ttl_ms} />,
          )}
          disabled={isSubmitting}
          fullWidth
          inputProps={{ min: 0, step: 1 }}
          label={t("defaultTtlLabel")}
          variant="outlined"
          type="number"
        />

        <Box display="flex">
          <div>
            {/*"getFieldHelpers" can't be used as it requires "helperText" property to be present.*/}
            <Checkbox
              id="allow_user_cancel_workspace_jobs"
              name="allow_user_cancel_workspace_jobs"
              disabled={isSubmitting}
              checked={form.values.allow_user_cancel_workspace_jobs}
              onChange={form.handleChange}
            />
          </div>
          <Box>
            <Typography variant="h6" style={{ fontSize: 14 }}>
              {t("allowUserCancelWorkspaceJobsLabel")}
            </Typography>
            <Typography variant="caption" color="textSecondary">
              {t("allowUserCancelWorkspaceJobsNotice")}
            </Typography>
          </Box>
        </Box>
      </Stack>

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </form>
  )
}
