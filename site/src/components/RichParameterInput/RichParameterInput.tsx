import FormControlLabel from "@material-ui/core/FormControlLabel"
import Radio from "@material-ui/core/Radio"
import RadioGroup from "@material-ui/core/RadioGroup"
import { makeStyles } from "@material-ui/core/styles"
import TextField, { TextFieldProps } from "@material-ui/core/TextField"
import { Stack } from "components/Stack/Stack"
import { FC, useState } from "react"
import { TemplateVersionParameter } from "../../api/typesGenerated"
import { colors } from "theme/colors"
import { MemoizedMarkdown } from "components/Markdown/Markdown"
import { MultiTextField } from "components/MultiTextField/MultiTextField"

const isBoolean = (parameter: TemplateVersionParameter) => {
  return parameter.type === "bool"
}

export interface ParameterLabelProps {
  id: string
  parameter: TemplateVersionParameter
}

const ParameterLabel: FC<ParameterLabelProps> = ({ id, parameter }) => {
  const styles = useStyles()
  const hasDescription = parameter.description && parameter.description !== ""

  return (
    <label htmlFor={id}>
      <Stack direction="row" alignItems="center">
        {parameter.icon && (
          <span className={styles.labelIconWrapper}>
            <img
              className={styles.labelIcon}
              alt="Parameter icon"
              src={parameter.icon}
            />
          </span>
        )}

        {hasDescription ? (
          <Stack spacing={0.5}>
            <span className={styles.labelCaption}>{parameter.name}</span>
            <span className={styles.labelPrimary}>
              <MemoizedMarkdown>{parameter.description}</MemoizedMarkdown>
            </span>
          </Stack>
        ) : (
          <span className={styles.labelPrimary}>{parameter.name}</span>
        )}
      </Stack>
    </label>
  )
}

export type RichParameterInputProps = TextFieldProps & {
  index: number
  parameter: TemplateVersionParameter
  onChange: (value: string) => void
  initialValue?: string
  id: string
}

export const RichParameterInput: FC<RichParameterInputProps> = ({
  index,
  disabled,
  onChange,
  parameter,
  initialValue,
  ...fieldProps
}) => {
  const styles = useStyles()

  return (
    <Stack direction="column" spacing={0.75}>
      <ParameterLabel id={fieldProps.id} parameter={parameter} />
      <div className={styles.input}>
        <RichParameterField
          {...fieldProps}
          index={index}
          disabled={disabled}
          onChange={onChange}
          parameter={parameter}
          initialValue={initialValue}
        />
      </div>
    </Stack>
  )
}

const RichParameterField: React.FC<RichParameterInputProps> = ({
  disabled,
  onChange,
  parameter,
  initialValue,
  ...props
}) => {
  const [parameterValue, setParameterValue] = useState(initialValue)
  const styles = useStyles()

  if (isBoolean(parameter)) {
    return (
      <RadioGroup
        defaultValue={parameterValue}
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

  if (parameter.options.length > 0) {
    return (
      <RadioGroup
        defaultValue={parameterValue}
        onChange={(event) => {
          onChange(event.target.value)
        }}
      >
        {parameter.options.map((option) => (
          <FormControlLabel
            disabled={disabled}
            key={option.name}
            value={option.value}
            control={<Radio color="primary" size="small" disableRipple />}
            label={
              <span className={styles.radioOption}>
                {option.icon && (
                  <img
                    className={styles.optionIcon}
                    alt="Parameter icon"
                    src={option.icon}
                    style={{
                      pointerEvents: "none",
                    }}
                  />
                )}
                {option.name}
              </span>
            }
          />
        ))}
      </RadioGroup>
    )
  }

  if (parameter.type === "list(string)") {
    let values: string[] = []

    if (parameterValue) {
      try {
        values = JSON.parse(parameterValue) as string[]
      } catch (e) {
        console.error("Error parsing list(string) parameter", e)
      }
    }

    return (
      <MultiTextField
        label={props.label as string}
        values={values}
        onChange={(values) => {
          try {
            const value = JSON.stringify(values)
            setParameterValue(value)
            onChange(value)
          } catch (e) {
            console.error("Error on change of list(string) parameter", e)
          }
        }}
      />
    )
  }

  // A text field can technically handle all cases!
  // As other cases become more prominent (like filtering for numbers),
  // we should break this out into more finely scoped input fields.
  return (
    <TextField
      {...props}
      type={parameter.type}
      size="small"
      disabled={disabled}
      required={parameter.required}
      placeholder={parameter.default_value}
      value={parameterValue}
      onChange={(event) => {
        setParameterValue(event.target.value)
        onChange(event.target.value)
      }}
    />
  )
}

const optionIconSize = 20

const useStyles = makeStyles((theme) => ({
  label: {
    marginBottom: theme.spacing(0.5),
  },
  labelCaption: {
    fontSize: 14,
    color: theme.palette.text.secondary,
  },
  labelPrimary: {
    fontSize: 16,
    color: theme.palette.text.primary,
    fontWeight: 600,

    "& p": {
      margin: 0,
      lineHeight: "20px", // Keep the same as ParameterInput
    },
  },
  labelImmutable: {
    marginTop: theme.spacing(0.5),
    marginBottom: theme.spacing(0.5),
    color: colors.yellow[7],
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
  labelIconWrapper: {
    width: theme.spacing(2.5),
    height: theme.spacing(2.5),
    display: "block",
  },
  labelIcon: {
    width: "100%",
    height: "100%",
    objectFit: "contain",
  },
  radioOption: {
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1.5),
  },
  optionIcon: {
    maxHeight: optionIconSize,
    width: optionIconSize,
  },
}))
