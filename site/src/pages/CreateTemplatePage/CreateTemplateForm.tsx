import Checkbox from "@material-ui/core/Checkbox"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import {
  ParameterSchema,
  ProvisionerJobLog,
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
} from "util/formUtils"
import { CreateTemplateData } from "xServices/createTemplate/createTemplateXService"
import * as Yup from "yup"
import { WorkspaceBuildLogs } from "components/WorkspaceBuildLogs/WorkspaceBuildLogs"
import { HelpTooltip, HelpTooltipText } from "components/Tooltips/HelpTooltip"
import { LazyIconField } from "components/IconField/LazyIconField"
import { VariableInput } from "./VariableInput"
import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/HorizontalForm/HorizontalForm"
import camelCase from "lodash/camelCase"
import capitalize from "lodash/capitalize"

const validationSchema = Yup.object({
  name: nameValidator("Name"),
  display_name: templateDisplayNameValidator("Display name"),
})

const defaultInitialValues: CreateTemplateData = {
  name: "",
  display_name: "",
  description: "",
  icon: "",
  default_ttl_hours: 24,
  allow_user_cancel_workspace_jobs: false,
}

const getInitialValues = (starterTemplate?: TemplateExample) => {
  if (!starterTemplate) {
    return defaultInitialValues
  }

  return {
    ...defaultInitialValues,
    name: starterTemplate.id,
    display_name: starterTemplate.name,
    icon: starterTemplate.icon,
    description: starterTemplate.description,
  }
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
}

export const CreateTemplateForm: FC<CreateTemplateFormProps> = ({
  onCancel,
  onSubmit,
  starterTemplate,
  parameters,
  variables,
  isSubmitting,
  upload,
  error,
  jobError,
  logs,
}) => {
  const styles = useStyles()
  const form = useFormik<CreateTemplateData>({
    initialValues: getInitialValues(starterTemplate),
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<CreateTemplateData>(form, error)
  const { t } = useTranslation("createTemplatePage")

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
          <TextField
            {...getFieldHelpers("default_ttl_hours")}
            disabled={isSubmitting}
            onChange={onChangeTrimmed(form)}
            fullWidth
            label={t("form.fields.autoStop")}
            variant="outlined"
            type="number"
            helperText={t("form.helperText.autoStop")}
          />
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
      {parameters && (
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
      {variables && (
        <FormSection title="Variables" description="Template variables">
          <FormFields>
            {variables.map((variable, index) => (
              <VariableInput
                variable={variable}
                disabled={isSubmitting}
                key={variable.name}
                onChange={async (value) => {
                  await form.setFieldValue("user_variable_values." + index, {
                    name: variable.name,
                    value: value,
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
