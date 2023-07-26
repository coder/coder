import {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated"
import { useTranslation } from "react-i18next"
import * as Yup from "yup"

export const selectInitialRichParametersValues = (
  templateParameters?: TemplateVersionParameter[],
  defaultBuildParameters?: WorkspaceBuildParameter[],
): WorkspaceBuildParameter[] => {
  const defaults: WorkspaceBuildParameter[] = []
  if (!templateParameters) {
    return defaults
  }

  templateParameters.forEach((parameter) => {
    let parameterValue = parameter.default_value

    if (parameter.options.length > 0) {
      parameterValue = parameterValue ?? parameter.options[0].value
      const validValues = parameter.options.map((option) => option.value)

      if (defaultBuildParameters) {
        const defaultBuildParameter = defaultBuildParameters.find(
          (p) => p.name === parameter.name,
        )

        // We don't want invalid values from default parameters to be set
        if (
          defaultBuildParameter &&
          validValues.includes(defaultBuildParameter.value)
        ) {
          parameterValue = defaultBuildParameter?.value
        }
      }

      const buildParameter: WorkspaceBuildParameter = {
        name: parameter.name,
        value: parameterValue,
      }
      defaults.push(buildParameter)
      return
    }

    if (parameter.ephemeral) {
      parameterValue = parameter.default_value
    }

    if (defaultBuildParameters) {
      const buildParameter = defaultBuildParameters.find(
        (p) => p.name === parameter.name,
      )

      if (buildParameter) {
        parameterValue = buildParameter?.value
      }
    }

    const buildParameter: WorkspaceBuildParameter = {
      name: parameter.name,
      value: parameterValue || "",
    }
    defaults.push(buildParameter)
  })
  return defaults
}

export const useValidationSchemaForRichParameters = (
  ns: string,
  templateParameters?: TemplateVersionParameter[],
  lastBuildParameters?: WorkspaceBuildParameter[],
): Yup.AnySchema => {
  const { t } = useTranslation(ns)

  if (!templateParameters) {
    return Yup.object()
  }

  return Yup.array()
    .of(
      Yup.object().shape({
        name: Yup.string().required(),
        value: Yup.string().test("verify with template", (val, ctx) => {
          const name = ctx.parent.name
          const templateParameter = templateParameters.find(
            (parameter) => parameter.name === name,
          )
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
                    })
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
                    })
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
                    })
                  }
                }

                if (
                  templateParameter.validation_monotonic &&
                  lastBuildParameters
                ) {
                  const lastBuildParameter = lastBuildParameters.find(
                    (last) => last.name === name,
                  )
                  if (lastBuildParameter) {
                    switch (templateParameter.validation_monotonic) {
                      case "increasing":
                        if (Number(lastBuildParameter.value) > Number(val)) {
                          return ctx.createError({
                            path: ctx.path,
                            message: t("validationNumberNotIncreasing", {
                              last: lastBuildParameter.value,
                            }).toString(),
                          })
                        }
                        break
                      case "decreasing":
                        if (Number(lastBuildParameter.value) < Number(val)) {
                          return ctx.createError({
                            path: ctx.path,
                            message: t("validationNumberNotDecreasing", {
                              last: lastBuildParameter.value,
                            }).toString(),
                          })
                        }
                        break
                    }
                  }
                }
                break
              case "string":
                {
                  if (
                    !templateParameter.validation_regex ||
                    templateParameter.validation_regex.length === 0
                  ) {
                    return true
                  }

                  const regex = new RegExp(templateParameter.validation_regex)
                  if (val && !regex.test(val)) {
                    return ctx.createError({
                      path: ctx.path,
                      message: t("validationPatternNotMatched", {
                        error: templateParameter.validation_error,
                        pattern: templateParameter.validation_regex,
                      }).toString(),
                    })
                  }
                }
                break
            }
          }
          return true
        }),
      }),
    )
    .required()
}

export const workspaceBuildParameterValue = (
  workspaceBuildParameters: WorkspaceBuildParameter[],
  parameter: TemplateVersionParameter,
): string => {
  const buildParameter = workspaceBuildParameters.find((buildParameter) => {
    return buildParameter.name === parameter.name
  })
  return (buildParameter && buildParameter.value) || ""
}

export const getInitialParameterValues = (
  templateParameters: TemplateVersionParameter[],
  buildParameters: WorkspaceBuildParameter[],
) => {
  return templateParameters.map((parameter) => {
    const buildParameter = buildParameters.find(
      (p) => p.name === parameter.name,
    )
    if (!buildParameter || parameter.ephemeral) {
      return {
        name: parameter.name,
        value: parameter.default_value,
      }
    }
    return buildParameter
  })
}
