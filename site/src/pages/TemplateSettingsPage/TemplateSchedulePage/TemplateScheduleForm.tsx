import TextField from "@material-ui/core/TextField"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { FormikTouched, useFormik } from "formik"
import { FC } from "react"
import { getFormHelpers } from "utils/formUtils"
import * as Yup from "yup"
import i18next from "i18next"
import { useTranslation } from "react-i18next"
import { Maybe } from "components/Conditionals/Maybe"
import { FormSection, HorizontalForm, FormFooter } from "components/Form/Form"
import { Stack } from "components/Stack/Stack"
import { makeStyles } from "@material-ui/core/styles"
import Link from "@material-ui/core/Link"
import Checkbox from "@material-ui/core/Checkbox"

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

const MAX_TTL_DAYS = 7
const MS_HOUR_CONVERSION = 3600000

export const getValidationSchema = (): Yup.AnyObjectSchema =>
  Yup.object({
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
    allow_user_autostart: Yup.boolean(),
    allow_user_autostop: Yup.boolean(),
  })

export interface TemplateScheduleForm {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  error?: unknown
  allowAdvancedScheduling: boolean
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateMeta>
}

export const TemplateScheduleForm: FC<TemplateScheduleForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
  allowAdvancedScheduling,
  isSubmitting,
  initialTouched,
}) => {
  const { t: commonT } = useTranslation("common")
  const validationSchema = getValidationSchema()
  const form = useFormik<UpdateTemplateMeta>({
    initialValues: {
      // on display, convert from ms => hours
      default_ttl_ms: template.default_ttl_ms / MS_HOUR_CONVERSION,
      // the API ignores this value, but to avoid tripping up validation set
      // it to zero if the user can't set the field.
      max_ttl_ms: allowAdvancedScheduling
        ? template.max_ttl_ms / MS_HOUR_CONVERSION
        : 0,
      allow_user_autostart: template.allow_user_autostart,
      allow_user_autostop: template.allow_user_autostop,
    },
    validationSchema,
    onSubmit: (formData) => {
      // on submit, convert from hours => ms
      onSubmit({
        default_ttl_ms: formData.default_ttl_ms
          ? formData.default_ttl_ms * MS_HOUR_CONVERSION
          : undefined,
        max_ttl_ms: formData.max_ttl_ms
          ? formData.max_ttl_ms * MS_HOUR_CONVERSION
          : undefined,
        allow_user_autostart: formData.allow_user_autostart,
        allow_user_autostop: formData.allow_user_autostop,
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
              allowAdvancedScheduling ? (
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
            disabled={isSubmitting || !allowAdvancedScheduling}
            fullWidth
            inputProps={{ min: 0, step: 1 }}
            label={t("maxTtlLabel")}
            variant="outlined"
            type="number"
          />
        </Stack>
      </FormSection>

      <FormSection
        title="Allow users scheduling"
        description="Allow users to set custom autostart and autostop scheduling options for workspaces created from this template."
      >
        <Stack direction="column">
          <Stack direction="row" alignItems="center">
            <Checkbox
              id="allow_user_autostart"
              size="small"
              color="primary"
              disabled={isSubmitting || !allowAdvancedScheduling}
              onChange={async () => {
                await form.setFieldValue(
                  "allow_user_autostart",
                  !form.values.allow_user_autostart,
                )
              }}
              name="allow_user_autostart"
              checked={form.values.allow_user_autostart}
            />
            <Stack spacing={0.5}>
              <strong>
                Allow users to autostart workspaces on a schedule.
              </strong>
            </Stack>
          </Stack>
          <Stack direction="row" alignItems="center">
            <Checkbox
              id="allow-user-autostop"
              size="small"
              color="primary"
              disabled={isSubmitting || !allowAdvancedScheduling}
              onChange={async () => {
                await form.setFieldValue(
                  "allow_user_autostop",
                  !form.values.allow_user_autostop,
                )
              }}
              name="allow_user_autostop"
              checked={form.values.allow_user_autostop}
            />
            <Stack spacing={0.5}>
              <strong>
                Allow users to customize autostop duration for workspaces.
              </strong>
              <span className={styles.optionDescription}>
                Workspaces will always use the default TTL if this is set.
                Regardless of this setting, workspaces can only stay on for the
                max lifetime.
              </span>
            </Stack>
          </Stack>
        </Stack>
      </FormSection>

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  )
}

const useStyles = makeStyles((theme) => ({
  ttlFields: {
    width: "100%",
  },
  optionDescription: {
    fontSize: 12,
    color: theme.palette.text.secondary,
  },
}))
