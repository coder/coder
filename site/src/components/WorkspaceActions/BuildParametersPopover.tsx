import {
  TemplateVersionParameter,
  WorkspaceBuildParameter,
} from "api/typesGenerated"
import { RichParameterInput } from "components/RichParameterInput/RichParameterInput"
import { useFormik } from "formik"
import { getFormHelpers } from "utils/formUtils"
import { getInitialParameterValues } from "utils/richParameters"

export const BuildParametersForm = ({
  ephemeralParameters,
  buildParameters,
}: {
  ephemeralParameters: TemplateVersionParameter[]
  buildParameters: WorkspaceBuildParameter[]
}) => {
  const form = useFormik({
    initialValues: {
      rich_parameter_values: getInitialParameterValues(
        ephemeralParameters,
        buildParameters,
      ),
    },
    onSubmit: () => {},
  })
  const getFieldHelpers = getFormHelpers(form)

  return (
    <form action="">
      {ephemeralParameters.map((parameter, index) => {
        return (
          <RichParameterInput
            {...getFieldHelpers("rich_parameter_values[" + index + "].value")}
            key={parameter.name}
            parameter={parameter}
            initialValue=""
            index={index}
            onChange={async (value) => {
              await form.setFieldValue(`rich_parameter_values[${index}]`, {
                name: parameter.name,
                value: value,
              })
            }}
          />
        )
      })}
    </form>
  )
}
