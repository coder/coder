import { TemplateVersionParameter } from "api/typesGenerated";
import { FormFields, FormSection } from "components/Form/Form";
import {
  RichParameterInput,
  RichParameterInputProps,
} from "components/RichParameterInput/RichParameterInput";
import { ComponentProps, FC } from "react";
import { AutofillSource } from "utils/richParameters";

export type TemplateParametersSectionProps = {
  templateParameters: TemplateVersionParameter[];
  autofillSources?: Record<string, AutofillSource>;
  getInputProps: (
    parameter: TemplateVersionParameter,
    index: number,
  ) => Omit<RichParameterInputProps, "parameter" | "index">;
} & Pick<ComponentProps<typeof FormSection>, "classes">;

export const TemplateParametersSection: FC<TemplateParametersSectionProps> = ({
  templateParameters,
  getInputProps,
  autofillSources,
  ...formSectionProps
}) => {
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
                    autofillSource={
                      autofillSources && autofillSources[parameter.name]
                    }
                  />
                ),
            )}
          </FormFields>
        </FormSection>
      )}
    </>
  );
};
