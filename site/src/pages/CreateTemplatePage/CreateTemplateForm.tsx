import Checkbox from "@material-ui/core/Checkbox"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import {
  ParameterSchema,
  ProvisionerJobLog,
  TemplateExample,
} from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
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
import { nameValidator, getFormHelpers, onChangeTrimmed } from "util/formUtils"
import { CreateTemplateData } from "xServices/createTemplate/createTemplateXService"
import * as Yup from "yup"
import { WorkspaceBuildLogs } from "components/WorkspaceBuildLogs/WorkspaceBuildLogs"
import { HelpTooltip, HelpTooltipText } from "components/Tooltips/HelpTooltip"
import { LazyIconField } from "components/IconField/LazyIconField"

const validationSchema = Yup.object({
  name: nameValidator("Name"),
  display_name: Yup.string().optional(),
  description: Yup.string().optional(),
  icon: Yup.string().optional(),
  default_ttl_hours: Yup.number(),
  allow_user_cancel_workspace_jobs: Yup.boolean(),
  parameter_values_by_name: Yup.object().optional(),
})

const defaultInitialValues: CreateTemplateData = {
  name: "",
  display_name: "",
  description: "",
  icon: "",
  default_ttl_hours: 24,
  allow_user_cancel_workspace_jobs: false,
  parameter_values_by_name: undefined,
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

interface CreateTemplateFormProps {
  starterTemplate?: TemplateExample
  error?: unknown
  parameters?: ParameterSchema[]
  isSubmitting: boolean
  onCancel: () => void
  onSubmit: (data: CreateTemplateData) => void
  upload: TemplateUploadProps
  jobError?: string
  logs?: ProvisionerJobLog[]
}

export const CreateTemplateForm: FC<CreateTemplateFormProps> = ({
  starterTemplate,
  error,
  parameters,
  isSubmitting,
  onCancel,
  onSubmit,
  upload,
  jobError,
  logs,
}) => {
  const styles = useStyles()
  const formFooterStyles = useFormFooterStyles()
  const form = useFormik<CreateTemplateData>({
    initialValues: getInitialValues(starterTemplate),
    validationSchema,
    onSubmit,
  })
  const getFieldHelpers = getFormHelpers<CreateTemplateData>(form, error)
  const { t } = useTranslation("createTemplatePage")

  return (
    <form onSubmit={form.handleSubmit}>
      <Stack direction="column" spacing={10} className={styles.formSections}>
        {/* General info */}
        <div className={styles.formSection}>
          <div className={styles.formSectionInfo}>
            <h2 className={styles.formSectionInfoTitle}>
              {t("form.generalInfo.title")}
            </h2>
            <p className={styles.formSectionInfoDescription}>
              {t("form.generalInfo.description")}
            </p>
          </div>

          <Stack direction="column" className={styles.formSectionFields}>
            {starterTemplate ? (
              <SelectedTemplate template={starterTemplate} />
            ) : (
              <TemplateUpload {...upload} />
            )}

            <TextField
              {...getFieldHelpers("name")}
              disabled={isSubmitting}
              onChange={onChangeTrimmed(form)}
              autoFocus
              fullWidth
              label={t("form.fields.name")}
              variant="outlined"
            />
          </Stack>
        </div>

        {/* Display info  */}
        <div className={styles.formSection}>
          <div className={styles.formSectionInfo}>
            <h2 className={styles.formSectionInfoTitle}>
              {t("form.displayInfo.title")}
            </h2>
            <p className={styles.formSectionInfoDescription}>
              {t("form.displayInfo.description")}
            </p>
          </div>

          <Stack direction="column" className={styles.formSectionFields}>
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
          </Stack>
        </div>

        {/* Schedule */}
        <div className={styles.formSection}>
          <div className={styles.formSectionInfo}>
            <h2 className={styles.formSectionInfoTitle}>
              {t("form.schedule.title")}
            </h2>
            <p className={styles.formSectionInfoDescription}>
              {t("form.schedule.description")}
            </p>
          </div>

          <Stack direction="column" className={styles.formSectionFields}>
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
          </Stack>
        </div>

        {/* Operations */}
        <div className={styles.formSection}>
          <div className={styles.formSectionInfo}>
            <h2 className={styles.formSectionInfoTitle}>
              {t("form.operations.title")}
            </h2>
            <p className={styles.formSectionInfoDescription}>
              {t("form.operations.description")}
            </p>
          </div>

          <Stack direction="column" className={styles.formSectionFields}>
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
          </Stack>
        </div>

        {/* Parameters */}
        {parameters && (
          <div className={styles.formSection}>
            <div className={styles.formSectionInfo}>
              <h2 className={styles.formSectionInfoTitle}>
                {t("form.parameters.title")}
              </h2>
              <p className={styles.formSectionInfoDescription}>
                {t("form.parameters.description")}
              </p>
            </div>

            <Stack direction="column" className={styles.formSectionFields}>
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
            </Stack>
          </div>
        )}

        {jobError && (
          <Stack>
            <div className={styles.error}>
              <h5 className={styles.errorTitle}>Error during provisioning</h5>
              <p className={styles.errorDescription}>
                Looks like we found an error during the template provisioning.
                You can see the logs bellow.
              </p>

              <code className={styles.errorDetails}>{jobError}</code>
            </div>

            <WorkspaceBuildLogs logs={logs ?? []} />
          </Stack>
        )}

        <FormFooter
          styles={formFooterStyles}
          onCancel={onCancel}
          isLoading={isSubmitting}
          submitLabel={jobError ? "Retry" : "Create template"}
        />
      </Stack>
    </form>
  )
}

const useStyles = makeStyles((theme) => ({
  formSections: {
    [theme.breakpoints.down("sm")]: {
      gap: theme.spacing(8),
    },
  },

  formSection: {
    display: "flex",
    alignItems: "flex-start",
    gap: theme.spacing(15),

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      gap: theme.spacing(2),
    },
  },

  formSectionInfo: {
    width: 312,
    flexShrink: 0,
    position: "sticky",
    top: theme.spacing(3),

    [theme.breakpoints.down("sm")]: {
      width: "100%",
      position: "initial",
    },
  },

  formSectionInfoTitle: {
    fontSize: 20,
    color: theme.palette.text.primary,
    fontWeight: 400,
    margin: 0,
    marginBottom: theme.spacing(1),
  },

  formSectionInfoDescription: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    lineHeight: "160%",
    margin: 0,
  },

  formSectionFields: {
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

const useFormFooterStyles = makeStyles((theme) => ({
  button: {
    minWidth: theme.spacing(23),

    [theme.breakpoints.down("sm")]: {
      width: "100%",
    },
  },
  footer: {
    display: "flex",
    alignItems: "center",
    justifyContent: "flex-start",
    flexDirection: "row-reverse",
    gap: theme.spacing(2),

    [theme.breakpoints.down("sm")]: {
      flexDirection: "column",
      gap: theme.spacing(1),
    },
  },
}))
