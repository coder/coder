import Checkbox from "@mui/material/Checkbox"
import { makeStyles } from "@mui/styles"
import TextField from "@mui/material/TextField"
import {
  ProvisionerJobLog,
  Template,
  TemplateExample,
  TemplateVersionVariable,
} from "api/typesGenerated"
import { Stack } from "components/Stack/Stack"
import {
  TemplateUpload,
  TemplateUploadProps,
} from "pages/CreateTemplatePage/TemplateUpload"
import { useFormik } from "formik"
import { SelectedTemplate } from "pages/CreateWorkspacePage/SelectedTemplate"
import { FC, useEffect } from "react"
import { useTranslation } from "react-i18next"
import {
  nameValidator,
  getFormHelpers,
  onChangeTrimmed,
  templateDisplayNameValidator,
} from "utils/formUtils"
import { CreateTemplateData } from "xServices/createTemplate/createTemplateXService"
import * as Yup from "yup"
import { WorkspaceBuildLogs } from "components/WorkspaceBuildLogs/WorkspaceBuildLogs"
import { HelpTooltip, HelpTooltipText } from "components/Tooltips/HelpTooltip"
import { LazyIconField } from "components/IconField/LazyIconField"
import { Maybe } from "components/Conditionals/Maybe"
import i18next from "i18next"
import Link from "@mui/material/Link"
import {
  HorizontalForm,
  FormSection,
  FormFields,
  FormFooter,
} from "components/Form/Form"
import camelCase from "lodash/camelCase"
import capitalize from "lodash/capitalize"
import { VariableInput } from "./VariableInput"
import { docs } from "utils/docs"
import {
  AutostopRequirementDaysHelperText,
  AutostopRequirementWeeksHelperText,
} from "pages/TemplateSettingsPage/TemplateSchedulePage/TemplateScheduleForm/AutostopRequirementHelperText"
import MenuItem from "@mui/material/MenuItem"

const MAX_DESCRIPTION_CHAR_LIMIT = 128
const MAX_TTL_DAYS = 30

const TTLHelperText = ({
  ttl,
  translationName,
}: {
  ttl?: number
  translationName: string
}) => {
  const { t } = useTranslation("createTemplatePage")
  const count = typeof ttl !== "number" ? 0 : ttl
  return (
    // no helper text if ttl is negative - error will show once field is considered touched
    <Maybe condition={count >= 0}>
      <span>{t(translationName, { count })}</span>
    </Maybe>
  )
}

const validationSchema = Yup.object({
  name: nameValidator(
    i18next.t("form.fields.name", { ns: "createTemplatePage" }),
  ),
  display_name: templateDisplayNameValidator(
    i18next.t("form.fields.displayName", {
      ns: "createTemplatePage",
    }),
  ),
  description: Yup.string().max(
    MAX_DESCRIPTION_CHAR_LIMIT,
    "Please enter a description that is less than or equal to 128 characters.",
  ),
  icon: Yup.string().optional(),
  default_ttl_hours: Yup.number()
    .integer()
    .min(0, "Default time until autostop must not be less than 0.")
    .max(
      24 * MAX_TTL_DAYS /* 30 days in hours */,
      "Please enter a limit that is less than or equal to 720 hours (30 days).",
    ),
  max_ttl_hours: Yup.number()
    .integer()
    .min(0, "Maximum time until autostop must not be less than 0.")
    .max(
      24 * MAX_TTL_DAYS /* 30 days in hours */,
      "Please enter a limit that is less than or equal to 720 hours (30 days).",
    ),
  autostop_requirement_days_of_week: Yup.string().required(),
  autostop_requirement_weeks: Yup.number().required().min(1).max(16),
})

const defaultInitialValues: CreateTemplateData = {
  name: "",
  display_name: "",
  description: "",
  icon: "",
  default_ttl_hours: 24,
  // max_ttl is an enterprise-only feature, and the server ignores the value if
  // you are not licensed. We hide the form value based on entitlements.
  //
  // The maximum value is 30 days but we default to 7 days as it's a much more
  // sensible value for most teams.
  max_ttl_hours: 24 * 7,
  // autostop_requirement is an enterprise-only feature, and the server ignores
  // the value if you are not licensed. We hide the form value based on
  // entitlements.
  //
  // Default to requiring restart every Sunday in the user's quiet hours in the
  // user's timezone.
  autostop_requirement_days_of_week: "sunday",
  autostop_requirement_weeks: 1,
  allow_user_cancel_workspace_jobs: false,
  allow_user_autostart: false,
  allow_user_autostop: false,
  allow_everyone_group_access: true,
}

type GetInitialValuesParams = {
  fromExample?: TemplateExample
  fromCopy?: Template
  variables?: TemplateVersionVariable[]
  allowAdvancedScheduling: boolean
}

const getInitialValues = ({
  fromExample,
  fromCopy,
  allowAdvancedScheduling,
  variables,
}: GetInitialValuesParams) => {
  let initialValues = defaultInitialValues

  if (!allowAdvancedScheduling) {
    initialValues = {
      ...initialValues,
      max_ttl_hours: 0,
    }
  }

  if (fromExample) {
    initialValues = {
      ...initialValues,
      name: fromExample.id,
      display_name: fromExample.name,
      icon: fromExample.icon,
      description: fromExample.description,
    }
  }

  if (fromCopy) {
    initialValues = {
      ...initialValues,
      ...fromCopy,
      name: `${fromCopy.name}-copy`,
      display_name: fromCopy.display_name
        ? `Copy of ${fromCopy.display_name}`
        : "",
    }
  }

  if (variables) {
    variables.forEach((variable) => {
      if (!initialValues.user_variable_values) {
        initialValues.user_variable_values = []
      }
      initialValues.user_variable_values.push({
        name: variable.name,
        value: variable.sensitive ? "" : variable.value,
      })
    })
  }

  return initialValues
}

export interface CreateTemplateFormProps {
  onCancel: () => void
  onSubmit: (data: CreateTemplateData) => void
  isSubmitting: boolean
  upload: TemplateUploadProps
  starterTemplate?: TemplateExample
  variables?: TemplateVersionVariable[]
  error?: unknown
  jobError?: string
  logs?: ProvisionerJobLog[]
  allowAdvancedScheduling: boolean
  copiedTemplate?: Template
  allowDisableEveryoneAccess: boolean
  allowAutostopRequirement: boolean
}

export const CreateTemplateForm: FC<CreateTemplateFormProps> = ({
  onCancel,
  onSubmit,
  starterTemplate,
  copiedTemplate,
  variables,
  isSubmitting,
  upload,
  error,
  jobError,
  logs,
  allowAdvancedScheduling,
  allowDisableEveryoneAccess,
  allowAutostopRequirement,
}) => {
  const styles = useStyles()
  const form = useFormik<CreateTemplateData>({
    initialValues: getInitialValues({
      allowAdvancedScheduling,
      fromExample: starterTemplate,
      fromCopy: copiedTemplate,
      variables,
    }),
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<CreateTemplateData>(form, error)
  const { t } = useTranslation("createTemplatePage")
  const { t: commonT } = useTranslation("common")

  useEffect(() => {
    if (error) {
      window.scrollTo(0, 0)
    }
  }, [error])

  useEffect(() => {
    if (jobError) {
      window.scrollTo(0, document.body.scrollHeight)
    }
  }, [logs, jobError])

  // Set autostop_requirement weeks to 1 when days_of_week is set to "off" or
  // "daily". Technically you can set weeks to a different value in the backend
  // and it will work, but this is a UX decision so users don't set days=daily
  // and weeks=2 and get confused when workspaces only restart daily during
  // every second week.
  //
  // We want to set the value to 1 when the user selects "off" or "daily"
  // because the input gets disabled so they can't change it to 1 themselves.
  const {
    values: { autostop_requirement_days_of_week },
    setFieldValue,
  } = form
  useEffect(() => {
    if (!["saturday", "sunday"].includes(autostop_requirement_days_of_week)) {
      // This is async but we don't really need to await the value.
      void setFieldValue("autostop_requirement_weeks", 1)
    }
  }, [autostop_requirement_days_of_week, setFieldValue])

  return (
    <HorizontalForm onSubmit={form.handleSubmit}>
      {/* General info */}
      <FormSection
        title="General"
        description="The name is used to identify the template in URLs and the API."
      >
        <FormFields>
          {starterTemplate ? (
            <SelectedTemplate template={starterTemplate} />
          ) : copiedTemplate ? (
            <SelectedTemplate template={copiedTemplate} />
          ) : (
            <TemplateUpload
              {...upload}
              onUpload={async (file) => {
                await fillNameAndDisplayWithFilename(file.name, form)
                upload.onUpload(file)
              }}
            />
          )}

          <TextField
            {...getFieldHelpers("name")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            autoFocus
            fullWidth
            required
            label={t("form.fields.name")}
          />
        </FormFields>
      </FormSection>

      {/* Display info  */}
      <FormSection
        title="Display"
        description="A friendly name, description, and icon to help developers identify your template."
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("display_name")}
            disabled={isSubmitting}
            fullWidth
            label={t("form.fields.displayName")}
          />

          <TextField
            {...getFieldHelpers("description")}
            disabled={isSubmitting}
            rows={5}
            multiline
            fullWidth
            label={t("form.fields.description")}
          />

          <LazyIconField
            {...getFieldHelpers("icon")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            fullWidth
            label={t("form.fields.icon")}
            onPickEmoji={(value) => form.setFieldValue("icon", value)}
          />
        </FormFields>
      </FormSection>

      {/* Schedule */}
      <FormSection
        title="Schedule"
        description="Define when workspaces created from this template automatically stop."
      >
        <FormFields>
          <Stack direction="row" className={styles.ttlFields}>
            <TextField
              {...getFieldHelpers(
                "default_ttl_hours",
                <TTLHelperText
                  translationName="form.helperText.defaultTTLHelperText"
                  ttl={form.values.default_ttl_hours}
                />,
              )}
              disabled={isSubmitting}
              onChange={onChangeTrimmed(form)}
              fullWidth
              label={t("form.fields.autostop")}
              type="number"
            />

            {!allowAutostopRequirement && (
              <TextField
                {...getFieldHelpers(
                  "max_ttl_hours",
                  allowAdvancedScheduling ? (
                    <TTLHelperText
                      translationName="form.helperText.maxTTLHelperText"
                      ttl={form.values.max_ttl_hours}
                    />
                  ) : (
                    <>
                      {commonT("licenseFieldTextHelper")}{" "}
                      <Link href={docs("/enterprise")}>
                        {commonT("learnMore")}
                      </Link>
                      .
                    </>
                  ),
                )}
                disabled={isSubmitting || !allowAdvancedScheduling}
                fullWidth
                label={t("form.fields.maxTTL")}
                type="number"
              />
            )}
          </Stack>

          {allowAutostopRequirement && (
            <Stack direction="row" className={styles.ttlFields}>
              <TextField
                {...getFieldHelpers(
                  "autostop_requirement_days_of_week",
                  <AutostopRequirementDaysHelperText
                    days={form.values.autostop_requirement_days_of_week}
                  />,
                )}
                disabled={isSubmitting}
                fullWidth
                select
                value={form.values.autostop_requirement_days_of_week}
                label={t("form.fields.autostopRequirementDays")}
              >
                <MenuItem key="off" value="off">
                  {t("form.fields.autostopRequirementDays_off")}
                </MenuItem>
                <MenuItem key="daily" value="daily">
                  {t("form.fields.autostopRequirementDays_daily")}
                </MenuItem>
                <MenuItem key="saturday" value="saturday">
                  {t("form.fields.autostopRequirementDays_saturday")}
                </MenuItem>
                <MenuItem key="sunday" value="sunday">
                  {t("form.fields.autostopRequirementDays_sunday")}
                </MenuItem>
              </TextField>

              <TextField
                {...getFieldHelpers(
                  "autostop_requirement_weeks",
                  <AutostopRequirementWeeksHelperText
                    days={form.values.autostop_requirement_days_of_week}
                    weeks={form.values.autostop_requirement_weeks}
                  />,
                )}
                disabled={
                  isSubmitting ||
                  !["saturday", "sunday"].includes(
                    form.values.autostop_requirement_days_of_week || "",
                  )
                }
                fullWidth
                inputProps={{ min: 1, max: 16, step: 1 }}
                label={t("form.fields.autostopRequirementWeeks")}
                type="number"
              />
            </Stack>
          )}

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
                  Allow users to automatically start workspaces on a schedule.
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
                name="allow-user-autostop"
                checked={form.values.allow_user_autostop}
              />
              <Stack spacing={0.5}>
                <strong>
                  Allow users to customize autostop duration for workspaces.
                </strong>
                <span className={styles.optionHelperText}>
                  Workspaces will always use the default TTL if this is set.
                  Regardless of this setting, workspaces can only stay on for
                  the max TTL.
                </span>
              </Stack>
            </Stack>
          </Stack>
        </FormFields>
      </FormSection>

      {/* Permissions */}
      <FormSection
        title="Permissions"
        description="Regulate actions allowed on workspaces created from this template."
      >
        <Stack direction="column">
          <FormFields>
            <label htmlFor="allow_user_cancel_workspace_jobs">
              <Stack direction="row" spacing={1}>
                <Checkbox
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
                    <strong>{t("form.fields.allowUsersToCancel")}</strong>

                    <HelpTooltip>
                      <HelpTooltipText>
                        {t("form.tooltip.allowUsersToCancel")}
                      </HelpTooltipText>
                    </HelpTooltip>
                  </Stack>
                  <span className={styles.optionHelperText}>
                    {t("form.helperText.allowUsersToCancel")}
                  </span>
                </Stack>
              </Stack>
            </label>
          </FormFields>
          <FormFields>
            <label htmlFor="allow_everyone_group_access">
              <Stack direction="row" spacing={1}>
                <Checkbox
                  id="allow_everyone_group_access"
                  name="allow_everyone_group_access"
                  disabled={isSubmitting || !allowDisableEveryoneAccess}
                  checked={form.values.allow_everyone_group_access}
                  onChange={form.handleChange}
                />

                <Stack direction="column" spacing={0.5}>
                  <Stack
                    direction="row"
                    alignItems="center"
                    spacing={0.5}
                    className={styles.optionText}
                  >
                    <strong>Allow everyone to use the template</strong>

                    <HelpTooltip>
                      <HelpTooltipText>
                        If unchecked, only users with the &apos;template
                        admin&apos; and &apos;owner&apos; role can use this
                        template until the permissions are updated. Navigate to{" "}
                        <strong>
                          Templates &gt; Select a template &gt; Settings &gt;
                          Permissions
                        </strong>{" "}
                        to update permissions.
                      </HelpTooltipText>
                    </HelpTooltip>
                  </Stack>
                  <span className={styles.optionHelperText}>
                    This setting requires an enterprise license for the&nbsp;
                    <Link href={docs("/admin/rbac")}>
                      &apos;Template RBAC&apos;
                    </Link>{" "}
                    feature to customize permissions.
                  </span>
                </Stack>
              </Stack>
            </label>
          </FormFields>
        </Stack>
      </FormSection>

      {/* Variables */}
      {variables && variables.length > 0 && (
        <FormSection
          title="Variables"
          description="Input variables allow you to customize templates without altering their source code."
        >
          <FormFields>
            {variables.map((variable, index) => (
              <VariableInput
                defaultValue={variable.value}
                variable={variable}
                disabled={isSubmitting}
                key={variable.name}
                onChange={async (value) => {
                  await form.setFieldValue("user_variable_values." + index, {
                    name: variable.name,
                    value,
                  })
                }}
              />
            ))}
          </FormFields>
        </FormSection>
      )}

      {jobError && (
        <Stack>
          <div className={styles.error}>
            <h5 className={styles.errorTitle}>Error during provisioning</h5>
            <p className={styles.errorDescription}>
              Looks like we found an error during the template provisioning. You
              can see the logs bellow.
            </p>

            <code className={styles.errorDetails}>{jobError}</code>
          </div>

          <WorkspaceBuildLogs logs={logs ?? []} />
        </Stack>
      )}

      <FormFooter
        onCancel={onCancel}
        isLoading={isSubmitting}
        submitLabel={jobError ? "Retry" : "Create template"}
      />
    </HorizontalForm>
  )
}

const fillNameAndDisplayWithFilename = async (
  filename: string,
  form: ReturnType<typeof useFormik<CreateTemplateData>>,
) => {
  const [name, _extension] = filename.split(".")
  await Promise.all([
    form.setFieldValue(
      "name",
      // Camel case will remove special chars and spaces
      camelCase(name).toLowerCase(),
    ),
    form.setFieldValue("display_name", capitalize(name)),
  ])
}

const useStyles = makeStyles((theme) => ({
  ttlFields: {
    width: "100%",
  },

  optionText: {
    fontSize: theme.spacing(2),
    color: theme.palette.text.primary,
  },

  optionHelperText: {
    fontSize: theme.spacing(1.5),
    color: theme.palette.text.secondary,
  },

  error: {
    padding: theme.spacing(3),
    borderRadius: theme.spacing(1),
    background: theme.palette.background.paper,
    border: `1px solid ${theme.palette.error.main}`,
  },

  errorTitle: {
    fontSize: 16,
    margin: 0,
  },

  errorDescription: {
    margin: 0,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(0.5),
  },

  errorDetails: {
    display: "block",
    marginTop: theme.spacing(1),
    color: theme.palette.error.light,
    fontSize: theme.spacing(2),
  },
}))
