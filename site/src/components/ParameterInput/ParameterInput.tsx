import FormControlLabel from "@material-ui/core/FormControlLabel"
import Paper from "@material-ui/core/Paper"
import Radio from "@material-ui/core/Radio"
import RadioGroup from "@material-ui/core/RadioGroup"
import { lighten, makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import React from "react"
import { ParameterSchema } from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"

export interface ParameterInputProps {
  disabled?: boolean
  schema: ParameterSchema
  onChange: (value: string) => void
}

export const ParameterInput: React.FC<ParameterInputProps> = ({ disabled, onChange, schema }) => {
  const styles = useStyles()
  return (
    <Paper className={styles.paper}>
      <div className={styles.title}>
        <h2>var.{schema.name}</h2>
        {schema.description && <span>{schema.description}</span>}
      </div>
      <div className={styles.input}>
        <ParameterField disabled={disabled} onChange={onChange} schema={schema} />
      </div>
    </Paper>
  )
}

const ParameterField: React.FC<ParameterInputProps> = ({ disabled, onChange, schema }) => {
  if (schema.validation_contains.length > 0) {
    return (
      <RadioGroup
        defaultValue={schema.default_source_value}
        onChange={(event) => {
          onChange(event.target.value)
        }}
      >
        {schema.validation_contains.map((item) => (
          <FormControlLabel
            disabled={disabled}
            key={item}
            value={item}
            control={<Radio color="primary" size="small" disableRipple />}
            label={item}
          />
        ))}
      </RadioGroup>
    )
  }

  // A text field can technically handle all cases!
  // As other cases become more prominent (like filtering for numbers),
  // we should break this out into more finely scoped input fields.
  return (
    <TextField
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
  paper: {
    display: "flex",
    flexDirection: "column",
    fontFamily: MONOSPACE_FONT_FAMILY,
  },
  title: {
    background: lighten(theme.palette.background.default, 0.1),
    borderBottom: `1px solid ${theme.palette.divider}`,
    padding: theme.spacing(3),
    display: "flex",
    flexDirection: "column",
    "& h2": {
      margin: 0,
    },
    "& span": {
      paddingTop: theme.spacing(2),
    },
  },
  input: {
    padding: theme.spacing(3),
    display: "flex",
    flexDirection: "column",
    maxWidth: 480,
  },
}))
