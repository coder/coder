import FormControlLabel from "@material-ui/core/FormControlLabel"
import Radio from "@material-ui/core/Radio"
import RadioGroup from "@material-ui/core/RadioGroup"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { FC } from "react"
import { ParameterSchema } from "../../api/typesGenerated"
import { MONOSPACE_FONT_FAMILY } from "../../theme/constants"

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
    <div className={styles.root}>
      <div className={styles.title}>
        <h2>var.{schema.name}</h2>
        {schema.description && <span>{schema.description}</span>}
      </div>
      <div className={styles.input}>
        <ParameterField disabled={disabled} onChange={onChange} schema={schema} />
      </div>
    </div>
  )
}

const ParameterField: React.FC<React.PropsWithChildren<ParameterInputProps>> = ({
  disabled,
  onChange,
  schema,
}) => {
  if (schema.validation_contains && schema.validation_contains.length > 0) {
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
  root: {
    display: "flex",
    flexDirection: "column",
    fontFamily: MONOSPACE_FONT_FAMILY,
    paddingTop: theme.spacing(2),
    paddingBottom: theme.spacing(2),
  },
  title: {
    display: "flex",
    flexDirection: "column",
    "& h2": {
      margin: 0,
    },
    "& span": {
      paddingTop: theme.spacing(1),
    },
  },
  input: {
    marginTop: theme.spacing(2),
    display: "flex",
    flexDirection: "column",
  },
}))
