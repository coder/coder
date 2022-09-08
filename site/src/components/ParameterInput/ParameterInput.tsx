import FormControlLabel from "@material-ui/core/FormControlLabel"
import MenuItem from "@material-ui/core/MenuItem"
import Radio from "@material-ui/core/Radio"
import RadioGroup from "@material-ui/core/RadioGroup"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { ParameterSchema } from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"

const isBoolean = (schema: ParameterSchema) => {
  return schema.validation_value_type === "bool"
}

const ParameterLabel: React.FC<{ schema: ParameterSchema }> = ({ schema }) => {
  const styles = useStyles()

  return (
    <label className={styles.label} htmlFor={schema.name}>
      <strong>var.{schema.name}</strong>
      {schema.description && <span className={styles.labelDescription}>{schema.description}</span>}
    </label>
  )
}

export interface ParameterInputProps {
  disabled?: boolean
  schema: ParameterSchema
  onChange: (value: string) => void
}

export const ParameterInput: FC<React.PropsWithChildren<ParameterInputProps>> = ({
  disabled,
  onChange,
  schema,
}) => {
  const styles = useStyles()

  return (
    <Stack direction="column" className={styles.root}>
      <ParameterLabel schema={schema} />
      <div className={styles.input}>
        <ParameterField disabled={disabled} onChange={onChange} schema={schema} />
      </div>
    </Stack>
  )
}

const ParameterField: React.FC<React.PropsWithChildren<ParameterInputProps>> = ({
  disabled,
  onChange,
  schema,
}) => {
  if (schema.validation_contains && schema.validation_contains.length > 0) {
    return (
      <TextField
        id={schema.name}
        size="small"
        defaultValue={schema.default_source_value}
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
      id={schema.name}
      size="small"
      disabled={disabled}
      placeholder={schema.default_source_value}
      onChange={(event) => {
        onChange(event.target.value)
      }}
    />
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    fontFamily: MONOSPACE_FONT_FAMILY,
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2),
  },
  label: {
    display: "flex",
    flexDirection: "column",
    fontSize: 21,
  },
  labelDescription: {
    fontSize: 14,
    marginTop: theme.spacing(1),
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
