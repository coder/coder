import FormControlLabel from "@material-ui/core/FormControlLabel"
import Radio from "@material-ui/core/Radio"
import RadioGroup from "@material-ui/core/RadioGroup"
import TextField from "@material-ui/core/TextField"
import { TemplateVersionVariable } from "api/typesGenerated"
import { FC, useState } from "react"
import { useTranslation } from "react-i18next"

export const SensitiveVariableHelperText = () => {
  const { t } = useTranslation("templateVariablesPage")
  return <span>{t("sensitiveVariableHelperText")}</span>
}

export interface TemplateVariableFieldProps {
  templateVersionVariable: TemplateVersionVariable
  initialValue: string
  disabled: boolean
  onChange: (value: string) => void
}

export const TemplateVariableField: FC<TemplateVariableFieldProps> = ({
  templateVersionVariable,
  initialValue,
  disabled,
  onChange,
  ...props
}) => {
  const [variableValue, setVariableValue] = useState(initialValue)
  if (isBoolean(templateVersionVariable)) {
    return (
      <RadioGroup
        defaultValue={variableValue}
        onChange={(event) => {
          onChange(event.target.value)
        }}
      >
        <FormControlLabel
          disabled={disabled}
          value="true"
          control={<Radio color="primary" size="small" disableRipple />}
          label="True"
        />
        <FormControlLabel
          disabled={disabled}
          value="false"
          control={<Radio color="primary" size="small" disableRipple />}
          label="False"
        />
      </RadioGroup>
    )
  }

  return (
    <TextField
      {...props}
      type={
        templateVersionVariable.type === "number"
          ? "number"
          : templateVersionVariable.sensitive
          ? "password"
          : "string"
      }
      disabled={disabled}
      autoFocus
      fullWidth
      label={templateVersionVariable.name}
      value={variableValue}
      placeholder={
        templateVersionVariable.sensitive
          ? ""
          : templateVersionVariable.default_value
      }
      onChange={(event) => {
        setVariableValue(event.target.value)
        onChange(event.target.value)
      }}
      variant="outlined"
    />
  )
}

const isBoolean = (variable: TemplateVersionVariable) => {
  return variable.type === "bool"
}
