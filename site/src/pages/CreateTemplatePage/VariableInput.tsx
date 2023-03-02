import FormControlLabel from "@material-ui/core/FormControlLabel"
import Radio from "@material-ui/core/Radio"
import RadioGroup from "@material-ui/core/RadioGroup"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { TemplateVersionVariable } from "../../api/typesGenerated"

const isBoolean = (variable: TemplateVersionVariable) => {
  return variable.type === "bool"
}

const VariableLabel: React.FC<{ variable: TemplateVersionVariable }> = ({
  variable,
}) => {
  const styles = useStyles()

  return (
    <label htmlFor={variable.name}>
      <span className={styles.labelName}>
        var.{variable.name}
        {!variable.required && " (optional)"}
      </span>
      <span className={styles.labelDescription}>{variable.description}</span>
    </label>
  )
}

export interface VariableInputProps {
  disabled?: boolean
  variable: TemplateVersionVariable
  onChange: (value: string) => void
  defaultValue?: string
}

export const VariableInput: FC<VariableInputProps> = ({
  disabled,
  onChange,
  variable,
  defaultValue,
}) => {
  const styles = useStyles()

  return (
    <Stack direction="column" spacing={0.75}>
      <VariableLabel variable={variable} />
      <div className={styles.input}>
        <VariableField
          disabled={disabled}
          onChange={onChange}
          variable={variable}
          defaultValue={defaultValue}
        />
      </div>
    </Stack>
  )
}

const VariableField: React.FC<VariableInputProps> = ({
  disabled,
  onChange,
  variable,
  defaultValue,
}) => {
  if (isBoolean(variable)) {
    return (
      <RadioGroup
        id={variable.name}
        defaultValue={variable.default_value}
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
      id={variable.name}
      size="small"
      disabled={disabled}
      placeholder={variable.sensitive ? "" : variable.default_value}
      required={variable.required}
      defaultValue={
        variable.sensitive ? "" : defaultValue ?? variable.default_value
      }
      onChange={(event) => {
        onChange(event.target.value)
      }}
      type={
        variable.type === "number"
          ? "number"
          : variable.sensitive
          ? "password"
          : "string"
      }
    />
  )
}

const useStyles = makeStyles((theme) => ({
  labelName: {
    fontSize: 14,
    color: theme.palette.text.secondary,
    display: "block",
    marginBottom: theme.spacing(0.5),
  },
  labelDescription: {
    fontSize: 16,
    color: theme.palette.text.primary,
    display: "block",
    fontWeight: 600,
  },
  input: {
    display: "flex",
    flexDirection: "column",
  },
  checkbox: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1),
  },
}))
