import TextField from "@material-ui/core/TextField"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
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
import {
  FormFields,
  FormSection,
  HorizontalForm,
  FormFooter,
} from "components/Form/Form"
import { Stack } from "components/Stack/Stack"
import Checkbox from "@material-ui/core/Checkbox"
import { HelpTooltip, HelpTooltipText } from "components/Tooltips/HelpTooltip"
import { makeStyles } from "@material-ui/core/styles"
import Link from "@material-ui/core/Link"

const TTLHelperText = ({
  ttl,
  translationName,
}: {
  ttl?: number
  translationName: string
}) => {
  const { t } = useTranslation("templateSettingsPage")
  const count = typeof ttl !== "number" ? 0 : ttl
  return (
    // no helper text if ttl is negative - error will show once field is considered touched
    <Maybe condition={count >= 0}>
      <span>{t(translationName, { count })}</span>
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
      .min(0, i18next.t("defaultTTLMinError", { ns: "templateSettingsPage" }))
      .max(
        24 * MAX_TTL_DAYS /* 7 days in hours */,
        i18next.t("defaultTTLMaxError", { ns: "templateSettingsPage" }),
      ),
    max_ttl_ms: Yup.number()
      .integer()
      .min(0, i18next.t("maxTTLMinError", { ns: "templateSettingsPage" }))
      .max(
        24 * MAX_TTL_DAYS /* 7 days in hours */,
        i18next.t("maxTTLMaxError", { ns: "templateSettingsPage" }),
      ),
    allow_user_cancel_workspace_jobs: Yup.boolean(),
  })

export interface TemplateSettingsForm {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  error?: unknown
  canSetMaxTTL: boolean
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateMeta>
}

export const TemplateSettingsForm: FC<TemplateSettingsForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
  canSetMaxTTL,
  isSubmitting,
  initialTouched,
}) => {
  const { t: commonT } = useTranslation("common")
  const validationSchema = getValidationSchema()
  const form: FormikContextType<UpdateTemplateMeta> =
    useFormik<UpdateTemplateMeta>({
      initialValues: {
        name: template.name,
        display_name: template.display_name,
        description: template.description,
        // on display, convert from ms => hours
        default_ttl_ms: template.default_ttl_ms / MS_HOUR_CONVERSION,
        // the API ignores this value, but to avoid tripping up validation set
        // it to zero if the user can't set the field.
        max_ttl_ms: canSetMaxTTL ? template.max_ttl_ms / MS_HOUR_CONVERSION : 0,
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
          max_ttl_ms: formData.max_ttl_ms
            ? formData.max_ttl_ms * MS_HOUR_CONVERSION
            : undefined,
        })
      },
      initialTouched,
    })
  const getFieldHelpers = getFormHelpers<UpdateTemplateMeta>(form, error)
  const { t } = useTranslation("templateSettingsPage")
  const styles = useStyles()

  return (
    <HorizontalForm
      onSubmit={form.handleSubmit}
      aria-label={t("formAriaLabel")}
    >
      <FormSection
        title={t("generalInfo.title")}
        description={t("generalInfo.description")}
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("name")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            autoFocus
            fullWidth
            label={t("nameLabel")}
            variant="outlined"
          />
        </FormFields>
      </FormSection>

      <FormSection
        title={t("displayInfo.title")}
        description={t("displayInfo.description")}
      >
        <FormFields>
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
            label={t("iconLabel")}
            variant="outlined"
            onPickEmoji={(value) => form.setFieldValue("icon", value)}
          />
        </FormFields>
      </FormSection>

      <FormSection
        title={t("schedule.title")}
        description={t("schedule.description")}
      >
        <Stack direction="row" className={styles.ttlFields}>
          <TextField
            {...getFieldHelpers(
              "default_ttl_ms",
              <TTLHelperText
                translationName="defaultTTLHelperText"
                ttl={form.values.default_ttl_ms}
              />,
            )}
            disabled={isSubmitting}
            fullWidth
            inputProps={{ min: 0, step: 1 }}
            label={t("defaultTtlLabel")}
            variant="outlined"
            type="number"
          />

          <TextField
            {...getFieldHelpers(
              "max_ttl_ms",
              canSetMaxTTL ? (
                <TTLHelperText
                  translationName="maxTTLHelperText"
                  ttl={form.values.max_ttl_ms}
                />
              ) : (
                <>
                  {commonT("licenseFieldTextHelper")}{" "}
                  <Link href="https://coder.com/docs/v2/latest/enterprise">
                    {commonT("learnMore")}
                  </Link>
                  .
                </>
              ),
            )}
            disabled={isSubmitting || !canSetMaxTTL}
            fullWidth
            inputProps={{ min: 0, step: 1 }}
            label={t("maxTtlLabel")}
            variant="outlined"
            type="number"
          />
        </Stack>
      </FormSection>

      <FormSection
        title={t("operations.title")}
        description={t("operations.description")}
      >
        <label htmlFor="allow_user_cancel_workspace_jobs">
          <Stack direction="row" spacing={1}>
            <Checkbox
              color="primary"
              id="allow_user_cancel_workspace_jobs"
              name="allow_user_cancel_workspace_jobs"
              disabled={isSubmitting}
              checked={form.values.allow_user_cancel_workspace_jobs}
              onChange={form.handleChange}
            />

            <Stack direction="column" spacing={0.5}>
              <Stack
                direction="row"
                alignItems="center"
                spacing={0.5}
                className={styles.optionText}
              >
                {t("allowUserCancelWorkspaceJobsLabel")}

                <HelpTooltip>
                  <HelpTooltipText>
                    {t("allowUserCancelWorkspaceJobsNotice")}
                  </HelpTooltipText>
                </HelpTooltip>
              </Stack>
              <span className={styles.optionHelperText}>
                {t("allowUsersCancelHelperText")}
              </span>
            </Stack>
          </Stack>
        </label>
      </FormSection>

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  )
}

const useStyles = makeStyles((theme) => ({
  optionText: {
    fontSize: theme.spacing(2),
    color: theme.palette.text.primary,
  },

  optionHelperText: {
    fontSize: theme.spacing(1.5),
    color: theme.palette.text.secondary,
  },

  ttlFields: {
    width: "100%",
  },
}))
