import FormControlLabel from "@mui/material/FormControlLabel"
import MenuItem from "@mui/material/MenuItem"
import Radio from "@mui/material/Radio"
import RadioGroup from "@mui/material/RadioGroup"
import { makeStyles } from "@mui/styles"
import TextField from "@mui/material/TextField"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { ParameterSchema } from "../../api/typesGenerated"

const isBoolean = (schema: ParameterSchema) => {
  return schema.validation_value_type === "bool"
}

const ParameterLabel: React.FC<{ schema: ParameterSchema }> = ({ schema }) => {
  const styles = useStyles()

  if (schema.name && schema.description) {
    return (
      <label htmlFor={schema.name}>
        <span className={styles.labelName}>var.{schema.name}</span>
        <span className={styles.labelDescription}>{schema.description}</span>
      </label>
    )
  }

  return (
    <label htmlFor={schema.name}>
      <span className={styles.labelDescription}>var.{schema.name}</span>
    </label>
  )
}

export interface ParameterInputProps {
  disabled?: boolean
  schema: ParameterSchema
  onChange: (value: string) => void
  defaultValue?: string
}

export const ParameterInput: FC<ParameterInputProps> = ({
  disabled,
  onChange,
  schema,
  defaultValue,
}) => {
  const styles = useStyles()

  return (
    <Stack direction="column" spacing={2}>
      <ParameterLabel schema={schema} />
      <div className={styles.input}>
        <ParameterField
          disabled={disabled}
          onChange={onChange}
          schema={schema}
          defaultValue={defaultValue}
        />
      </div>
    </Stack>
  )
}

const ParameterField: React.FC<ParameterInputProps> = ({
  disabled,
  onChange,
  schema,
  defaultValue,
}) => {
  if (schema.validation_contains && schema.validation_contains.length > 0) {
    return (
      <TextField
        id={schema.name}
        defaultValue={defaultValue ?? schema.default_source_value}
        placeholder={schema.default_source_value}
        disabled={disabled}
        onChange={(event) => {
          onChange(event.target.value)
        }}
        select
        fullWidth
      >
        {schema.validation_contains.map((item) => (
          <MenuItem key={item} value={item}>
            {item}
          </MenuItem>
        ))}
      </TextField>
    )
  }

  if (isBoolean(schema)) {
    return (
      <RadioGroup
        id={schema.name}
        defaultValue={schema.default_source_value}
        onChange={(event) => {
          onChange(event.target.value)
        }}
      >
        <FormControlLabel
          disabled={disabled}
          value="true"
          control={<Radio />}
          label="True"
        />
        <FormControlLabel
          disabled={disabled}
          value="false"
          control={<Radio />}
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
      id={schema.name}
      disabled={disabled}
      placeholder={schema.default_source_value}
      defaultValue={defaultValue ?? schema.default_source_value}
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
  },
  labelDescription: {
    fontSize: 16,
    color: theme.palette.text.primary,
    display: "block",
    fontWeight: 600,
    lineHeight: "24px", // Keep the same as ParameterInput
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
