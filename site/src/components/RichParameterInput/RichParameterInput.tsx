import FormControlLabel from "@material-ui/core/FormControlLabel"
import Radio from "@material-ui/core/Radio"
import RadioGroup from "@material-ui/core/RadioGroup"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { TemplateVersionParameter } from "../../api/typesGenerated"

const isBoolean = (parameter: TemplateVersionParameter) => {
  return parameter.type === "bool"
}

const ParameterLabel: React.FC<{ parameter: TemplateVersionParameter }> = ({ parameter }) => {
  const styles = useStyles()

  if (parameter.name && parameter.description) {
    return (
      <label htmlFor={parameter.name}>
        <span className={styles.labelName}>{parameter.name}</span>
        <span className={styles.labelDescription}>{parameter.description}</span>
      </label>
    )
  }

  return (
    <label htmlFor={parameter.name}>
      <span className={styles.labelDescription}>{parameter.name}</span>
    </label>
  )
}

export interface RichParameterInputProps {
  disabled?: boolean
  parameter: TemplateVersionParameter
  onChange: (value: string) => void
  defaultValue?: string
}

export const RichParameterInput: FC<RichParameterInputProps> = ({
  disabled,
  onChange,
  parameter,
}) => {
  const styles = useStyles()

  return (
    <Stack direction="column" spacing={0.75}>
      <ParameterLabel parameter={parameter} />
      <div className={styles.input}>
        <RichParameterField
          disabled={disabled}
          onChange={onChange}
          parameter={parameter}
        />
      </div>
    </Stack>
  )
}

const RichParameterField: React.FC<RichParameterInputProps> = ({
  disabled,
  onChange,
  parameter
}) => {
  if (isBoolean(parameter)) {
    return (
      <RadioGroup
        id={parameter.name}
        defaultValue={parameter.default_value}
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

  // A text field can technically handle all cases!
  // As other cases become more prominent (like filtering for numbers),
  // we should break this out into more finely scoped input fields.
  return (
    <TextField
      id={parameter.name}
      type={parameter.type}
      size="small"
      disabled={disabled}
      placeholder={parameter.default_value}
      defaultValue={parameter.default_value}
      onChange={(event) => {
        onChange(event.target.value)
      }}
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
