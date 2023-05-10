import TextField from "@mui/material/TextField"
import { Template, UpdateTemplateMeta } from "api/typesGenerated"
import { FormikTouched, useFormik } from "formik"
import { FC, ChangeEvent } from "react"
import { getFormHelpers } from "utils/formUtils"
import * as Yup from "yup"
import i18next from "i18next"
import { useTranslation } from "react-i18next"
import { Maybe } from "components/Conditionals/Maybe"
import {
  FormSection,
  HorizontalForm,
  FormFooter,
  FormFields,
} from "components/Form/Form"
import { Stack } from "components/Stack/Stack"
import { makeStyles } from "@mui/styles"
import Link from "@mui/material/Link"
import Checkbox from "@mui/material/Checkbox"
import FormControlLabel from "@mui/material/FormControlLabel"
import Switch from "@mui/material/Switch"

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
const MS_DAY_CONVERSION = 86400000
const FAILURE_CLEANUP_DEFAULT = 7
const INACTIVITY_CLEANUP_DEFAULT = 180

export interface TemplateScheduleFormValues extends UpdateTemplateMeta {
  failure_cleanup_enabled: boolean
  inactivity_cleanup_enabled: boolean
}

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
    failure_ttl_ms: Yup.number()
      .min(0, "Failure cleanup days must not be less than 0.")
      .test(
        "positive-if-enabled",
        "Failure cleanup days must be greater than zero when enabled.",
        function (value) {
          const parent = this.parent as TemplateScheduleFormValues
          if (parent.failure_cleanup_enabled) {
            return Boolean(value)
          } else {
            return true
          }
        },
      ),
    inactivity_ttl_ms: Yup.number()
      .min(0, "Inactivity cleanup days must not be less than 0.")
      .test(
        "positive-if-enabled",
        "Inactivity cleanup days must be greater than zero when enabled.",
        function (value) {
          const parent = this.parent as TemplateScheduleFormValues
          if (parent.inactivity_cleanup_enabled) {
            return Boolean(value)
          } else {
            return true
          }
        },
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
  allowWorkspaceActions: boolean
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<UpdateTemplateMeta>
}

export const TemplateScheduleForm: FC<TemplateScheduleForm> = ({
  template,
  onSubmit,
  onCancel,
  error,
  allowAdvancedScheduling,
  allowWorkspaceActions,
  isSubmitting,
  initialTouched,
}) => {
  const { t: commonT } = useTranslation("common")
  const validationSchema = getValidationSchema()
  const form = useFormik<TemplateScheduleFormValues>({
    initialValues: {
      // on display, convert from ms => hours
      default_ttl_ms: template.default_ttl_ms / MS_HOUR_CONVERSION,
      // the API ignores these values, but to avoid tripping up validation set
      // it to zero if the user can't set the field.
      max_ttl_ms: allowAdvancedScheduling
        ? template.max_ttl_ms / MS_HOUR_CONVERSION
        : 0,
      failure_ttl_ms: allowAdvancedScheduling
        ? template.failure_ttl_ms / MS_DAY_CONVERSION
        : 0,
      inactivity_ttl_ms: allowAdvancedScheduling
        ? template.inactivity_ttl_ms / MS_DAY_CONVERSION
        : 0,

      allow_user_autostart: template.allow_user_autostart,
      allow_user_autostop: template.allow_user_autostop,
      failure_cleanup_enabled:
        allowAdvancedScheduling && Boolean(template.failure_ttl_ms),
      inactivity_cleanup_enabled:
        allowAdvancedScheduling && Boolean(template.inactivity_ttl_ms),
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
        failure_ttl_ms: formData.failure_ttl_ms
          ? formData.failure_ttl_ms * MS_DAY_CONVERSION
          : undefined,
        inactivity_ttl_ms: formData.inactivity_ttl_ms
          ? formData.inactivity_ttl_ms * MS_DAY_CONVERSION
          : undefined,

        allow_user_autostart: formData.allow_user_autostart,
        allow_user_autostop: formData.allow_user_autostop,
      })
    },
    initialTouched,
  })
  const getFieldHelpers = getFormHelpers<TemplateScheduleFormValues>(
    form,
    error,
  )
  const { t } = useTranslation("templateSettingsPage")
  const styles = useStyles()

  const handleToggleFailureCleanup = async (e: ChangeEvent) => {
    form.handleChange(e)
    if (!form.values.failure_cleanup_enabled) {
      // fill failure_ttl_ms with defaults
      await form.setValues({
        ...form.values,
        failure_cleanup_enabled: true,
        failure_ttl_ms: FAILURE_CLEANUP_DEFAULT,
      })
    } else {
      // clear failure_ttl_ms
      await form.setValues({
        ...form.values,
        failure_cleanup_enabled: false,
        failure_ttl_ms: 0,
      })
    }
  }

  const handleToggleInactivityCleanup = async (e: ChangeEvent) => {
    form.handleChange(e)
    if (!form.values.inactivity_cleanup_enabled) {
      // fill inactivity_ttl_ms with defaults
      await form.setValues({
        ...form.values,
        inactivity_cleanup_enabled: true,
        inactivity_ttl_ms: INACTIVITY_CLEANUP_DEFAULT,
      })
    } else {
      // clear inactivity_ttl_ms
      await form.setValues({
        ...form.values,
        inactivity_cleanup_enabled: false,
        inactivity_ttl_ms: 0,
      })
    }
  }

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
      {allowAdvancedScheduling && allowWorkspaceActions && (
        <>
          <FormSection
            title="Failure Cleanup"
            description="When enabled, Coder will attempt to stop workspaces that are in a failed state after a specified number of days."
          >
            <FormFields>
              <FormControlLabel
                control={
                  <Switch
                    name="failureCleanupEnabled"
                    checked={form.values.failure_cleanup_enabled}
                    onChange={handleToggleFailureCleanup}
                  />
                }
                label="Enable Failure Cleanup"
              />
              <TextField
                {...getFieldHelpers(
                  "failure_ttl_ms",
                  <TTLHelperText
                    translationName="failureTTLHelperText"
                    ttl={form.values.failure_ttl_ms}
                  />,
                )}
                disabled={isSubmitting || !form.values.failure_cleanup_enabled}
                fullWidth
                inputProps={{ min: 0, step: "any" }}
                label="Time until cleanup (days)"
                type="number"
                aria-label="Failure Cleanup"
              />
            </FormFields>
          </FormSection>
          <FormSection
            title="Inactivity Cleanup"
            description="When enabled, Coder will automatically delete workspaces that are in an inactive state after a specified number of days."
          >
            <FormFields>
              <FormControlLabel
                control={
                  <Switch
                    name="inactivityCleanupEnabled"
                    checked={form.values.inactivity_cleanup_enabled}
                    onChange={handleToggleInactivityCleanup}
                  />
                }
                label="Enable Inactivity Cleanup"
              />
              <TextField
                {...getFieldHelpers(
                  "inactivity_ttl_ms",
                  <TTLHelperText
                    translationName="inactivityTTLHelperText"
                    ttl={form.values.inactivity_ttl_ms}
                  />,
                )}
                disabled={
                  isSubmitting || !form.values.inactivity_cleanup_enabled
                }
                fullWidth
                inputProps={{ min: 0, step: "any" }}
                label="Time until cleanup (days)"
                type="number"
                aria-label="Inactivity Cleanup"
              />
            </FormFields>
          </FormSection>
        </>
      )}
      <FormFooter
        onCancel={onCancel}
        isLoading={isSubmitting}
        submitDisabled={!form.isValid || !form.dirty}
      />
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
