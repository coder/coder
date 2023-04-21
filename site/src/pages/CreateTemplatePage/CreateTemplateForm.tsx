import Checkbox from "@material-ui/core/Checkbox"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import {
  ParameterSchema,
  ProvisionerJobLog,
  Template,
  TemplateExample,
  TemplateVersionVariable,
} from "api/typesGenerated"
import { ParameterInput } from "components/ParameterInput/ParameterInput"
import { Stack } from "components/Stack/Stack"
import {
  TemplateUpload,
  TemplateUploadProps,
} from "pages/CreateTemplatePage/TemplateUpload"
import { useFormik } from "formik"
import { SelectedTemplate } from "pages/CreateWorkspacePage/SelectedTemplate"
import { FC } from "react"
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
import Link from "@material-ui/core/Link"
import {
  HorizontalForm,
  FormSection,
  FormFields,
  FormFooter,
} from "components/Form/Form"
import camelCase from "lodash/camelCase"
import capitalize from "lodash/capitalize"
import { VariableInput } from "./VariableInput"

const MAX_DESCRIPTION_CHAR_LIMIT = 128
const MAX_TTL_DAYS = 7

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
    i18next.t("form.error.descriptionMax", { ns: "createTemplatePage" }),
  ),
  icon: Yup.string().optional(),
  default_ttl_hours: Yup.number()
    .integer()
    .min(
      0,
      i18next.t("form.error.defaultTTLMin", { ns: "templateSettingsPage" }),
    )
    .max(
      24 * MAX_TTL_DAYS /* 7 days in hours */,
      i18next.t("form.error.defaultTTLMax", { ns: "templateSettingsPage" }),
    ),
  max_ttl_hours: Yup.number()
    .integer()
    .min(0, i18next.t("form.error.maxTTLMin", { ns: "templateSettingsPage" }))
    .max(
      24 * MAX_TTL_DAYS /* 7 days in hours */,
      i18next.t("form.error.maxTTLMax", { ns: "templateSettingsPage" }),
    ),
})

const defaultInitialValues: CreateTemplateData = {
  name: "",
  display_name: "",
  description: "",
  icon: "",
  default_ttl_hours: 24,
  // max_ttl is an enterprise-only feature, and the server ignores the value if
  // you are not licensed. We hide the form value based on entitlements.
  max_ttl_hours: 24 * 7,
  allow_user_cancel_workspace_jobs: false,
  allow_user_autostart: false,
  allow_user_autostop: false,
}

type GetInitialValuesParams = {
  fromExample?: TemplateExample
  fromCopy?: Template
  parameters?: ParameterSchema[]
  variables?: TemplateVersionVariable[]
  allowAdvancedScheduling: boolean
}

const getInitialValues = ({
  fromExample,
  fromCopy,
  allowAdvancedScheduling,
  variables,
  parameters,
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

  if (parameters) {
    parameters.forEach((parameter) => {
      if (!initialValues.parameter_values_by_name) {
        initialValues.parameter_values_by_name = {}
      }
      initialValues.parameter_values_by_name[parameter.name] =
        parameter.default_source_value
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
  parameters?: ParameterSchema[]
  variables?: TemplateVersionVariable[]
  error?: unknown
  jobError?: string
  logs?: ProvisionerJobLog[]
  allowAdvancedScheduling: boolean
  copiedTemplate?: Template
}

export const CreateTemplateForm: FC<CreateTemplateFormProps> = ({
  onCancel,
  onSubmit,
  starterTemplate,
  copiedTemplate,
  parameters,
  variables,
  isSubmitting,
  upload,
  error,
  jobError,
  logs,
  allowAdvancedScheduling,
}) => {
  const styles = useStyles()
  const form = useFormik<CreateTemplateData>({
    initialValues: getInitialValues({
      allowAdvancedScheduling,
      fromExample: starterTemplate,
      fromCopy: copiedTemplate,
      variables,
      parameters,
    }),
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<CreateTemplateData>(form, error)
  const { t } = useTranslation("createTemplatePage")
  const { t: commonT } = useTranslation("common")

  return (
    <HorizontalForm onSubmit={form.handleSubmit}>
      {/* General info */}
      <FormSection
        title={t("form.generalInfo.title")}
        description={t("form.generalInfo.description")}
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
            variant="outlined"
          />
        </FormFields>
      </FormSection>

      {/* Display info  */}
      <FormSection
        title={t("form.displayInfo.title")}
        description={t("form.displayInfo.description")}
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("display_name")}
            disabled={isSubmitting}
            fullWidth
            label={t("form.fields.displayName")}
            variant="outlined"
          />

          <TextField
            {...getFieldHelpers("description")}
            disabled={isSubmitting}
            rows={5}
            multiline
            fullWidth
            label={t("form.fields.description")}
            variant="outlined"
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
        </FormFields>
      </FormSection>

      {/* Schedule */}
      <FormSection
        title={t("form.schedule.title")}
        description={t("form.schedule.description")}
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
              variant="outlined"
              type="number"
            />

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
                    <Link href="https://coder.com/docs/v2/latest/enterprise">
                      {commonT("learnMore")}
                    </Link>
                    .
                  </>
                ),
              )}
              disabled={isSubmitting || !allowAdvancedScheduling}
              fullWidth
              label={t("form.fields.maxTTL")}
              variant="outlined"
              type="number"
            />
          </Stack>
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

      {/* Operations */}
      <FormSection
        title={t("form.operations.title")}
        description={t("form.operations.description")}
      >
        <FormFields>
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
                  {t("form.fields.allowUsersToCancel")}

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
      </FormSection>

      {/* Parameters */}
      {parameters && parameters.length > 0 && (
        <FormSection
          title={t("form.parameters.title")}
          description={t("form.parameters.description")}
        >
          <FormFields>
            {parameters.map((schema) => (
              <ParameterInput
                schema={schema}
                disabled={isSubmitting}
                key={schema.id}
                onChange={async (value) => {
                  await form.setFieldValue(
                    `parameter_values_by_name.${schema.name}`,
                    value,
                  )
                }}
              />
            ))}
          </FormFields>
        </FormSection>
      )}

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
