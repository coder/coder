import TextField from "@material-ui/core/TextField"
import {
  CreateTemplateVersionRequest,
  TemplateVersion,
  TemplateVersionVariable,
  VariableValue,
} from "api/typesGenerated"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC } from "react"
import { getFormHelpers, onChangeTrimmed } from "util/formUtils"
import * as Yup from "yup"
import { useTranslation } from "react-i18next"
import { LazyIconField } from "components/IconField/LazyIconField"
import {
  FormFields,
  FormSection,
  HorizontalForm,
  FormFooter,
} from "components/HorizontalForm/HorizontalForm"
import { Stack } from "components/Stack/Stack"
import Checkbox from "@material-ui/core/Checkbox"
import { makeStyles } from "@material-ui/core/styles"

export const getValidationSchema = (): Yup.AnyObjectSchema => Yup.object()

export interface TemplateVariablesForm {
  templateVersion: TemplateVersion
  templateVariables: TemplateVersionVariable[]
  onSubmit: (data: CreateTemplateVersionRequest) => void
  onCancel: () => void
  isSubmitting: boolean
  error?: unknown
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<CreateTemplateVersionRequest>
}

export const TemplateVariablesForm: FC<TemplateVariablesForm> = ({
  templateVersion,
  templateVariables,
  onSubmit,
  onCancel,
  error,
  isSubmitting,
  initialTouched,
}) => {
  const validationSchema = getValidationSchema()
  const form: FormikContextType<CreateTemplateVersionRequest> =
    useFormik<CreateTemplateVersionRequest>({
      initialValues: {
        template_id: templateVersion.template_id,
        provisioner: "terraform",
        storage_method: "file",
        tags: {},
        // FIXME file_id: null,
        user_variable_values:
          selectInitialUserVariableValues(templateVariables),
      },
      validationSchema,
      onSubmit: onSubmit,
      initialTouched,
    })
  const getFieldHelpers = getFormHelpers<CreateTemplateVersionRequest>(
    form,
    error,
  )
  const { t } = useTranslation("TemplateVariablesPage")
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
        <TextField
          {...getFieldHelpers("default_ttl_ms")}
          disabled={isSubmitting}
          fullWidth
          inputProps={{ min: 0, step: 1 }}
          label={t("defaultTtlLabel")}
          variant="outlined"
          type="number"
        />
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
              checked={false}
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
}))

export const selectInitialUserVariableValues = (
  templateVariables: TemplateVersionVariable[],
): VariableValue[] => {
  const defaults: VariableValue[] = []
  templateVariables.forEach((templateVariable) => {
    if (
      templateVariable.value === "" &&
      templateVariable.default_value !== ""
    ) {
      defaults.push({
        name: templateVariable.name,
        value: templateVariable.default_value,
      })
      return
    }
    defaults.push({
      name: templateVariable.name,
      value: templateVariable.value,
    })
  })
  return defaults
}
