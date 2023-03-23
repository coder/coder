import TextField from "@material-ui/core/TextField"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC } from "react"
import { getFormHelpers } from "util/formUtils"
import * as Yup from "yup"
import i18next from "i18next"
import { useTranslation } from "react-i18next"
import { Maybe } from "components/Conditionals/Maybe"
import { FormSection, HorizontalForm, FormFooter } from "components/Form/Form"
import { Stack } from "components/Stack/Stack"
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
  })

export interface TemplateScheduleForm {
  template: Template
  onSubmit: (data: UpdateTemplateMeta) => void
  onCancel: () => void
  isSubmitting: boolean
  error?: unknown
  canSetMaxTTL: boolean
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateMeta>
}

export const TemplateScheduleForm: FC<TemplateScheduleForm> = ({
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
        // on display, convert from ms => hours
        default_ttl_ms: template.default_ttl_ms / MS_HOUR_CONVERSION,
        // the API ignores this value, but to avoid tripping up validation set
        // it to zero if the user can't set the field.
        max_ttl_ms: canSetMaxTTL ? template.max_ttl_ms / MS_HOUR_CONVERSION : 0,
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

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  )
}

const useStyles = makeStyles(() => ({
  ttlFields: {
    width: "100%",
  },
}))
