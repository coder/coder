import {
  CreateTemplateVersionRequest,
  TemplateVersion,
  TemplateVersionVariable,
  VariableValue,
} from "api/typesGenerated";
import { FormikContextType, FormikTouched, useFormik } from "formik";
import { FC } from "react";
import { getFormHelpers } from "utils/formUtils";
import * as Yup from "yup";
import {
  FormFields,
  FormSection,
  HorizontalForm,
  FormFooter,
} from "components/Form/Form";
import {
  SensitiveVariableHelperText,
  TemplateVariableField,
} from "./TemplateVariableField";

export interface TemplateVariablesForm {
  templateVersion: TemplateVersion;
  templateVariables: TemplateVersionVariable[];
  onSubmit: (data: CreateTemplateVersionRequest) => void;
  onCancel: () => void;
  isSubmitting: boolean;
  error?: unknown;
  // Helpful to show field errors on Storybook
  initialTouched?: FormikTouched<CreateTemplateVersionRequest>;
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
  const initialUserVariableValues =
    selectInitialUserVariableValues(templateVariables);
  const form: FormikContextType<CreateTemplateVersionRequest> =
    useFormik<CreateTemplateVersionRequest>({
      initialValues: {
        template_id: templateVersion.template_id,
        provisioner: "terraform",
        storage_method: "file",
        tags: templateVersion.job.tags,
        file_id: templateVersion.job.file_id,
        user_variable_values: initialUserVariableValues,
      },
      validationSchema: Yup.object({
        user_variable_values: ValidationSchemaForTemplateVariables(
          "templateVariablesPage",
          templateVariables,
        ),
      }),
      onSubmit,
      initialTouched,
    });
  const getFieldHelpers = getFormHelpers<CreateTemplateVersionRequest>(
    form,
    error,
  );

  return (
    <HorizontalForm
      onSubmit={form.handleSubmit}
      aria-label="Template variables"
    >
      {templateVariables.map((templateVariable, index) => {
        let fieldHelpers;
        if (templateVariable.sensitive) {
          fieldHelpers = getFieldHelpers(
            "user_variable_values[" + index + "].value",
            { helperText: <SensitiveVariableHelperText /> },
          );
        } else {
          fieldHelpers = getFieldHelpers(
            "user_variable_values[" + index + "].value",
          );
        }

        return (
          <FormSection
            key={templateVariable.name}
            title={templateVariable.name}
            description={templateVariable.description}
          >
            <FormFields>
              <TemplateVariableField
                {...fieldHelpers}
                templateVersionVariable={templateVariable}
                initialValue={initialUserVariableValues[index].value}
                disabled={isSubmitting}
                onChange={async (value) => {
                  await form.setFieldValue("user_variable_values." + index, {
                    name: templateVariable.name,
                    value: value,
                  });
                }}
              />
            </FormFields>
          </FormSection>
        );
      })}

      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  );
};

export const selectInitialUserVariableValues = (
  templateVariables: TemplateVersionVariable[],
): VariableValue[] => {
  const defaults: VariableValue[] = [];
  templateVariables.forEach((templateVariable) => {
    // Boolean variables must be always either "true" or "false"
    if (templateVariable.type === "bool" && templateVariable.value === "") {
      defaults.push({
        name: templateVariable.name,
        value: templateVariable.default_value,
      });
      return;
    }

    if (templateVariable.sensitive) {
      defaults.push({
        name: templateVariable.name,
        value: "",
      });
      return;
    }

    if (templateVariable.required && templateVariable.value === "") {
      defaults.push({
        name: templateVariable.name,
        value: templateVariable.default_value,
      });
      return;
    }

    defaults.push({
      name: templateVariable.name,
      value: templateVariable.value,
    });
  });
  return defaults;
};

const ValidationSchemaForTemplateVariables = (
  ns: string,
  templateVariables: TemplateVersionVariable[],
): Yup.AnySchema => {
  return Yup.array()
    .of(
      Yup.object().shape({
        name: Yup.string().required(),
        value: Yup.string().test("verify with template", (val, ctx) => {
          const name = ctx.parent.name;
          const templateVariable = templateVariables.find(
            (variable) => variable.name === name,
          );
          if (templateVariable && templateVariable.sensitive) {
            // It's possible that the secret is already stored in database,
            // so we can't properly verify the "required" condition.
            return true;
          }
          if (templateVariable && templateVariable.required) {
            if (!val || val.length === 0) {
              return ctx.createError({
                path: ctx.path,
                message: "Variable is required.",
              });
            }
          }
          return true;
        }),
      }),
    )
    .required();
};
