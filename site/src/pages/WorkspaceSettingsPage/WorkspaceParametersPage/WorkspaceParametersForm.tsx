import { useFormik } from "formik";
import { type FC } from "react";
import * as Yup from "yup";
import { Alert } from "components/Alert/Alert";
import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import {
  AutofillBuildParameter,
  getInitialRichParameterValues,
  useValidationSchemaForRichParameters,
} from "utils/richParameters";
import { getFormHelpers } from "utils/formUtils";
import type {
  TemplateVersionParameter,
  Workspace,
  WorkspaceBuildParameter,
} from "api/typesGenerated";

export type WorkspaceParametersFormValues = {
  rich_parameter_values: WorkspaceBuildParameter[];
};

interface WorkspaceParameterFormProps {
  workspace: Workspace;
  templateVersionRichParameters: TemplateVersionParameter[];
  autofillParams: AutofillBuildParameter[];
  isSubmitting: boolean;
  canChangeVersions: boolean;
  error: unknown;
  onCancel: () => void;
  onSubmit: (values: WorkspaceParametersFormValues) => void;
}

export const WorkspaceParametersForm: FC<WorkspaceParameterFormProps> = ({
  workspace,
  onCancel,
  onSubmit,
  templateVersionRichParameters,
  autofillParams,
  error,
  canChangeVersions,
  isSubmitting,
}) => {
  const form = useFormik<WorkspaceParametersFormValues>({
    onSubmit,
    initialValues: {
      rich_parameter_values: getInitialRichParameterValues(
        templateVersionRichParameters,
        autofillParams,
      ),
    },
    validationSchema: Yup.object({
      rich_parameter_values: useValidationSchemaForRichParameters(
        templateVersionRichParameters,
      ),
    }),
  });
  const getFieldHelpers = getFormHelpers<WorkspaceParametersFormValues>(
    form,
    error,
  );
  const hasEphemeralParameters = templateVersionRichParameters.some(
    (parameter) => parameter.ephemeral,
  );
  const hasNonEphemeralParameters = templateVersionRichParameters.some(
    (parameter) => !parameter.ephemeral,
  );

  const disabled =
    workspace.outdated &&
    workspace.template_require_active_version &&
    !canChangeVersions;

  return (
    <>
      {disabled && (
        <Alert severity="warning" css={{ marginBottom: 48 }}>
          The template for this workspace requires automatic updates. Update the
          workspace to edit parameters.
        </Alert>
      )}

      <HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
        {hasNonEphemeralParameters && (
          <FormSection
            title="Parameters"
            description="Settings used by your template"
          >
            <FormFields>
              {templateVersionRichParameters.map((parameter, index) =>
                // Since we are adding the values to the form based on the index
                // we can't filter them to not loose the right index position
                !parameter.ephemeral ? (
                  <RichParameterInput
                    {...getFieldHelpers(
                      "rich_parameter_values[" + index + "].value",
                    )}
                    disabled={isSubmitting || disabled || !parameter.mutable}
                    key={parameter.name}
                    onChange={async (value) => {
                      await form.setFieldValue(
                        "rich_parameter_values." + index,
                        {
                          name: parameter.name,
                          value: value,
                        },
                      );
                    }}
                    parameter={parameter}
                  />
                ) : null,
              )}
            </FormFields>
          </FormSection>
        )}
        {hasEphemeralParameters && (
          <FormSection
            title="Ephemeral Parameters"
            description="These parameters only apply for a single workspace start."
          >
            <FormFields>
              {templateVersionRichParameters.map((parameter, index) =>
                // Since we are adding the values to the form based on the index
                // we can't filter them to not loose the right index position
                parameter.mutable && parameter.ephemeral ? (
                  <RichParameterInput
                    {...getFieldHelpers(
                      "rich_parameter_values[" + index + "].value",
                    )}
                    disabled={isSubmitting || disabled}
                    key={parameter.name}
                    onChange={async (value) => {
                      await form.setFieldValue(
                        "rich_parameter_values." + index,
                        {
                          name: parameter.name,
                          value: value,
                        },
                      );
                    }}
                    parameter={parameter}
                  />
                ) : null,
              )}
            </FormFields>
          </FormSection>
        )}

        <FormFooter
          onCancel={onCancel}
          isLoading={isSubmitting}
          submitDisabled={disabled}
        />
      </HorizontalForm>
    </>
  );
};
