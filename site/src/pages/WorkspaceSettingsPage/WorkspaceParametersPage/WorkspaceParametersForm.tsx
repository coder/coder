import {
  FormFields,
  FormFooter,
  FormSection,
  HorizontalForm,
} from "components/Form/Form";
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput";
import { useFormik } from "formik";
import { FC } from "react";
import { useTranslation } from "react-i18next";
import {
  getInitialRichParameterValues,
  useValidationSchemaForRichParameters,
} from "utils/richParameters";
import * as Yup from "yup";
import { getFormHelpers } from "utils/formUtils";
import {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated";

export type WorkspaceParametersFormValues = {
  rich_parameter_values: WorkspaceBuildParameter[];
};

export const WorkspaceParametersForm: FC<{
  isSubmitting: boolean;
  templateVersionRichParameters: TemplateVersionParameter[];
  buildParameters: WorkspaceBuildParameter[];
  error: unknown;
  onCancel: () => void;
  onSubmit: (values: WorkspaceParametersFormValues) => void;
}> = ({
  onCancel,
  onSubmit,
  templateVersionRichParameters,
  buildParameters,
  error,
  isSubmitting,
}) => {
  const { t } = useTranslation("workspaceSettingsPage");

  const form = useFormik<WorkspaceParametersFormValues>({
    onSubmit,
    initialValues: {
      rich_parameter_values: getInitialRichParameterValues(
        templateVersionRichParameters,
        buildParameters,
      ),
    },
    validationSchema: Yup.object({
      rich_parameter_values: useValidationSchemaForRichParameters(
        "createWorkspacePage",
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
  const hasImmutableParameters = templateVersionRichParameters.some(
    (parameter) => !parameter.mutable,
  );

  return (
    <HorizontalForm onSubmit={form.handleSubmit} data-testid="form">
      {hasNonEphemeralParameters && (
        <FormSection
          title={t("parameters").toString()}
          description={t("parametersDescription").toString()}
        >
          <FormFields>
            {templateVersionRichParameters.map((parameter, index) =>
              // Since we are adding the values to the form based on the index
              // we can't filter them to not loose the right index position
              parameter.mutable && !parameter.ephemeral ? (
                <RichParameterInput
                  {...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  )}
                  disabled={isSubmitting}
                  key={parameter.name}
                  onChange={async (value) => {
                    await form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    });
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
                  disabled={isSubmitting}
                  key={parameter.name}
                  onChange={async (value) => {
                    await form.setFieldValue("rich_parameter_values." + index, {
                      name: parameter.name,
                      value: value,
                    });
                  }}
                  parameter={parameter}
                />
              ) : null,
            )}
          </FormFields>
        </FormSection>
      )}
      {/* They are displayed here only for visibility purposes */}
      {hasImmutableParameters && (
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
            {templateVersionRichParameters.map((parameter, index) =>
              !parameter.mutable ? (
                <RichParameterInput
                  disabled
                  {...getFieldHelpers(
                    "rich_parameter_values[" + index + "].value",
                  )}
                  key={parameter.name}
                  parameter={parameter}
                  onChange={() => {
                    throw new Error("Immutable parameters cannot be changed");
                  }}
                />
              ) : null,
            )}
          </FormFields>
        </FormSection>
      )}
      <FormFooter onCancel={onCancel} isLoading={isSubmitting} />
    </HorizontalForm>
  );
};
