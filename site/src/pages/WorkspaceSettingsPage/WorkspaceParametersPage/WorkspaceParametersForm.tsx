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
} from "utils/richParameters"
import * as Yup from "yup"
import { getFormHelpers } from "utils/formUtils"
import {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated"

export type WorkspaceParametersFormValues = {
  rich_parameter_values: WorkspaceBuildParameter[]
}

export const WorkspaceParametersForm: FC<{
  isSubmitting: boolean
  templateVersionRichParameters: TemplateVersionParameter[]
  buildParameters: WorkspaceBuildParameter[]
  error: unknown
  onCancel: () => void
  onSubmit: (values: WorkspaceParametersFormValues) => void
}> = ({
  onCancel,
  onSubmit,
  templateVersionRichParameters,
  buildParameters,
  error,
  isSubmitting,
}) => {
  const { t } = useTranslation("workspaceSettingsPage")
  const mutableParameters = templateVersionRichParameters.filter(
    (param) => param.mutable === true,
  )
  const immutableParameters = templateVersionRichParameters.filter(
    (param) => param.mutable === false,
  )
  const form = useFormik<WorkspaceParametersFormValues>({
    onSubmit,
    initialValues: {
      rich_parameter_values: mutableParameters.map((parameter) => {
        const buildParameter = buildParameters.find(
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
      rich_parameter_values: useValidationSchemaForRichParameters(
        "createWorkspacePage",
        templateVersionRichParameters,
      ),
    }),
  })
  const getFieldHelpers = getFormHelpers<WorkspaceParametersFormValues>(
    form,
    error,
  )

  return (
    <HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
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
                  buildParameters,
                  parameter,
                )}
              />
            ))}
          </FormFields>
        </FormSection>
      )}
      {/* They are displayed here only for visibility purposes */}
      {immutableParameters.length > 0 && (
        <FormSection
          title="Immutable parameters"
          description={
            <>
              These parameters are also provided by your Terraform configuration
              but they{" "}
              <strong>cannot be changed after creating the workspace.</strong>
            </>
          }
        >
          <FormFields>
            {immutableParameters.map((parameter, index) => (
              <RichParameterInput
                disabled
                {...getFieldHelpers(
                  "rich_parameter_values[" + index + "].value",
                )}
                index={index}
                key={parameter.name}
                onChange={async () => {
                  throw new Error(
                    "Cannot change immutable parameter after creation",
                  )
                }}
                parameter={parameter}
                initialValue={workspaceBuildParameterValue(
                  buildParameters,
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
