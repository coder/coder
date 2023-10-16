import { TemplateVersionParameter } from "api/typesGenerated";
import { FormSection, FormFields } from "components/Form/Form";
import {
  RichParameterInput,
  RichParameterInputProps,
} from "components/RichParameterInput/RichParameterInput";
import { ComponentProps, FC } from "react";

export type TemplateParametersSectionProps = {
  templateParameters: TemplateVersionParameter[];
  getInputProps: (
    parameter: TemplateVersionParameter,
    index: number,
  ) => Omit<RichParameterInputProps, "parameter" | "index">;
} & Pick<ComponentProps<typeof FormSection>, "classes">;

export const MutableTemplateParametersSection: FC<
  TemplateParametersSectionProps
> = ({ templateParameters, getInputProps, ...formSectionProps }) => {
  const hasMutableParameters =
    templateParameters.filter((p) => p.mutable).length > 0;

  return (
    <>
      {hasMutableParameters && (
        <FormSection
          {...formSectionProps}
          title="Parameters"
          description="Settings used by your template"
        >
          <FormFields>
            {templateParameters.map(
              (parameter, index) =>
                parameter.mutable && (
                  <RichParameterInput
                    {...getInputProps(parameter, index)}
                    key={parameter.name}
                    parameter={parameter}
                  />
                ),
            )}
          </FormFields>
        </FormSection>
      )}
    </>
  );
};

export const ImmutableTemplateParametersSection: FC<
  TemplateParametersSectionProps
> = ({ templateParameters, getInputProps, ...formSectionProps }) => {
  const hasImmutableParameters =
    templateParameters.filter((p) => !p.mutable).length > 0;

  return (
    <>
      {hasImmutableParameters && (
        <FormSection
          {...formSectionProps}
          title="Immutable parameters"
          description={
            <>
              These settings <strong>cannot be changed</strong> after creating
              the workspace.
            </>
          }
        >
          <FormFields>
            {templateParameters.map(
              (parameter, index) =>
                !parameter.mutable && (
                  <RichParameterInput
                    {...getInputProps(parameter, index)}
                    key={parameter.name}
                    parameter={parameter}
                  />
                ),
            )}
          </FormFields>
        </FormSection>
      )}
    </>
  );
};
