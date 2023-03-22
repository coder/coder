import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form"
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput"
import { useFormik } from "formik"
import { FC } from "react"
import { useTranslation } from "react-i18next"
import {
  useValidationSchemaForRichParameters,
  workspaceBuildParameterValue,
} from "util/richParameters"
import { WorkspaceSettings, WorkspaceSettingsFormValue } from "./data"
import * as Yup from "yup"
import { nameValidator, getFormHelpers, onChangeTrimmed } from "util/formUtils"
import TextField from "@material-ui/core/TextField"

export const WorkspaceSettingsForm: FC<{
  isSubmitting: boolean
  settings: WorkspaceSettings
  error: unknown
  onCancel: () => void
  onSubmit: (values: WorkspaceSettingsFormValue) => void
}> = ({ onCancel, onSubmit, settings, error, isSubmitting }) => {
  const { t } = useTranslation("workspaceSettingsPage")
  const mutableParameters = settings.templateVersionRichParameters.filter(
    (param) => param.mutable,
  )
  const form = useFormik<WorkspaceSettingsFormValue>({
    onSubmit,
    initialValues: {
      name: settings.workspace.name,
      rich_parameter_values: mutableParameters.map((parameter) => {
        const buildParameter = settings.buildParameters.find(
          (p) => p.name === parameter.name,
        )
        if (!buildParameter) {
          return {
            name: parameter.name,
            value: parameter.default_value,
          }
        }
        return buildParameter
      }),
    },
    validationSchema: Yup.object({
      name: nameValidator(t("nameLabel")),
      rich_parameter_values: useValidationSchemaForRichParameters(
        "createWorkspacePage",
        settings.templateVersionRichParameters,
      ),
    }),
  })
  const getFieldHelpers = getFormHelpers<WorkspaceSettingsFormValue>(
    form,
    error,
  )

  return (
    <HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
      <FormSection
        title={t("generalInfo")}
        description={t("generalInfoDescription")}
      >
        <FormFields>
          <TextField
            {...getFieldHelpers("name")}
            disabled={form.isSubmitting}
            onChange={onChangeTrimmed(form)}
            autoFocus
            fullWidth
            label={t("nameLabel")}
            variant="outlined"
          />
        </FormFields>
      </FormSection>
      {mutableParameters.length > 0 && (
        <FormSection
          title={t("parameters")}
          description={t("parametersDescription")}
        >
          <FormFields>
            {mutableParameters.map((parameter, index) => (
              <RichParameterInput
                {...getFieldHelpers(
                  "rich_parameter_values[" + index + "].value",
                )}
                disabled={isSubmitting}
                index={index}
                key={parameter.name}
                onChange={async (value) => {
                  await form.setFieldValue("rich_parameter_values." + index, {
                    name: parameter.name,
                    value: value,
                  })
                }}
                parameter={parameter}
                initialValue={workspaceBuildParameterValue(
                  settings.buildParameters,
                  parameter,
                )}
              />
            ))}
          </FormFields>
        </FormSection>
      )}
      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  )
}
