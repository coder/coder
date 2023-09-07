import {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated";
import { useTranslation } from "react-i18next";
import * as Yup from "yup";

export const getInitialRichParameterValues = (
  templateParameters: TemplateVersionParameter[],
  buildParameters?: WorkspaceBuildParameter[],
): WorkspaceBuildParameter[] => {
  return templateParameters.map((parameter) => {
    const existentBuildParameter = buildParameters?.find(
      (p) => p.name === parameter.name,
    );
    const shouldReturnTheDefaultValue =
      !existentBuildParameter ||
      !isValidValue(parameter, existentBuildParameter) ||
      parameter.ephemeral;
    if (shouldReturnTheDefaultValue) {
      return {
        name: parameter.name,
        value: parameter.default_value,
      };
    }
    return existentBuildParameter;
  });
};

const isValidValue = (
  templateParam: TemplateVersionParameter,
  buildParam: WorkspaceBuildParameter,
) => {
  if (templateParam.options.length > 0) {
    const validValues = templateParam.options.map((option) => option.value);
    return validValues.includes(buildParam.value);
  }

  return true;
};

export const useValidationSchemaForRichParameters = (
  ns: string,
  templateParameters?: TemplateVersionParameter[],
  lastBuildParameters?: WorkspaceBuildParameter[],
): Yup.AnySchema => {
  const { t } = useTranslation(ns);

  if (!templateParameters) {
    return Yup.object();
  }

  return Yup.array()
    .of(
      Yup.object().shape({
        name: Yup.string().required(),
        value: Yup.string().test("verify with template", (val, ctx) => {
          const name = ctx.parent.name;
          const templateParameter = templateParameters.find(
            (parameter) => parameter.name === name,
          );
          if (templateParameter) {
            switch (templateParameter.type) {
              case "number":
                if (
                  templateParameter.validation_min &&
                  !templateParameter.validation_max
                ) {
                  if (Number(val) < templateParameter.validation_min) {
                    return ctx.createError({
                      path: ctx.path,
                      message: t("validationNumberLesserThan", {
                        min: templateParameter.validation_min,
                      }).toString(),
                    });
                  }
                } else if (
                  !templateParameter.validation_min &&
                  templateParameter.validation_max
                ) {
                  if (templateParameter.validation_max < Number(val)) {
                    return ctx.createError({
                      path: ctx.path,
                      message: t("validationNumberGreaterThan", {
                        max: templateParameter.validation_max,
                      }).toString(),
                    });
                  }
                } else if (
                  templateParameter.validation_min &&
                  templateParameter.validation_max
                ) {
                  if (
                    Number(val) < templateParameter.validation_min ||
                    templateParameter.validation_max < Number(val)
                  ) {
                    return ctx.createError({
                      path: ctx.path,
                      message: t("validationNumberNotInRange", {
                        min: templateParameter.validation_min,
                        max: templateParameter.validation_max,
                      }).toString(),
                    });
                  }
                }

                if (
                  templateParameter.validation_monotonic &&
                  lastBuildParameters
                ) {
                  const lastBuildParameter = lastBuildParameters.find(
                    (last) => last.name === name,
                  );
                  if (lastBuildParameter) {
                    switch (templateParameter.validation_monotonic) {
                      case "increasing":
                        if (Number(lastBuildParameter.value) > Number(val)) {
                          return ctx.createError({
                            path: ctx.path,
                            message: t("validationNumberNotIncreasing", {
                              last: lastBuildParameter.value,
                            }).toString(),
                          });
                        }
                        break;
                      case "decreasing":
                        if (Number(lastBuildParameter.value) < Number(val)) {
                          return ctx.createError({
                            path: ctx.path,
                            message: t("validationNumberNotDecreasing", {
                              last: lastBuildParameter.value,
                            }).toString(),
                          });
                        }
                        break;
                    }
                  }
                }
                break;
              case "string":
                {
                  if (
                    !templateParameter.validation_regex ||
                    templateParameter.validation_regex.length === 0
                  ) {
                    return true;
                  }

                  const regex = new RegExp(templateParameter.validation_regex);
                  if (val && !regex.test(val)) {
                    return ctx.createError({
                      path: ctx.path,
                      message: t("validationPatternNotMatched", {
                        error: templateParameter.validation_error,
                        pattern: templateParameter.validation_regex,
                      }).toString(),
                    });
                  }
                }
                break;
            }
          }
          return true;
        }),
      }),
    )
    .required();
};
