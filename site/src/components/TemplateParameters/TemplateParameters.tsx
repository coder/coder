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
  const mutableParameters = templateParameters.filter((p) => p.mutable);

  if (mutableParameters.length === 0) {
    return null;
  }

  return (
    <FormSection
      {...formSectionProps}
      title="Parameters"
      description="Settings used by your template"
    >
      <FormFields>
        {mutableParameters.map((parameter, index) => (
          <RichParameterInput
            {...getInputProps(parameter, index)}
            key={parameter.name}
            parameter={parameter}
          />
        ))}
      </FormFields>
    </FormSection>
  );
};

export const ImmutableTemplateParametersSection: FC<
  TemplateParametersSectionProps
> = ({ templateParameters, getInputProps, ...formSectionProps }) => {
  const immutableParams = templateParameters.filter((p) => !p.mutable);

  if (immutableParams.length === 0) {
    return null;
  }

  return (
    <FormSection
      {...formSectionProps}
      title="Immutable parameters"
      description={
        <>
          These settings <strong>cannot be changed</strong> after creating the
          workspace.
        </>
      }
    >
      <FormFields>
        {immutableParams.map((parameter, index) => (
          <RichParameterInput
            {...getInputProps(parameter, index)}
            key={parameter.name}
            parameter={parameter}
          />
        ))}
      </FormFields>
    </FormSection>
  );
};
