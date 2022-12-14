import Checkbox from "@material-ui/core/Checkbox"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { ParameterSchema, TemplateExample } from "api/typesGenerated"
import { FormFooter } from "components/FormFooter/FormFooter"
import { IconField } from "components/IconField/IconField"
import { ParameterInput } from "components/ParameterInput/ParameterInput"
import { Stack } from "components/Stack/Stack"
import { useFormik } from "formik"
import { SelectedTemplate } from "pages/CreateWorkspacePage/SelectedTemplate"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import { nameValidator, getFormHelpers, onChangeTrimmed } from "util/formUtils"
import { CreateTemplateData } from "xServices/createTemplate/createTemplateXService"
import * as Yup from "yup"

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
}

export const CreateTemplateForm: FC<CreateTemplateFormProps> = ({
  starterTemplate,
  error,
  parameters,
  isSubmitting,
  onCancel,
  onSubmit,
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
            {starterTemplate && <SelectedTemplate template={starterTemplate} />}

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
              onChange={onChangeTrimmed(form)}
              fullWidth
              label={t("form.fields.displayName")}
              variant="outlined"
            />

            <TextField
              {...getFieldHelpers("description")}
              disabled={isSubmitting}
              onChange={onChangeTrimmed(form)}
              rows={5}
              multiline
              fullWidth
              label={t("form.fields.description")}
              variant="outlined"
            />

            <IconField
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
                  <span className={styles.optionText}>
                    {t("form.fields.allowUsersToCancel")}
                  </span>
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

        <FormFooter
          styles={formFooterStyles}
          onCancel={onCancel}
          isLoading={isSubmitting}
          submitLabel="Create template"
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
