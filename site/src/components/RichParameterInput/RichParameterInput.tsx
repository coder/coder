import FormControlLabel from "@mui/material/FormControlLabel"
import Radio from "@mui/material/Radio"
import RadioGroup from "@mui/material/RadioGroup"
import { makeStyles } from "@mui/styles"
import TextField, { TextFieldProps } from "@mui/material/TextField"
import { Stack } from "components/Stack/Stack"
import { FC } from "react"
import { TemplateVersionParameter } from "../../api/typesGenerated"
import { colors } from "theme/colors"
import { MemoizedMarkdown } from "components/Markdown/Markdown"
import { MultiTextField } from "components/MultiTextField/MultiTextField"
import Box from "@mui/material/Box"
import { Theme } from "@mui/material/styles"

const isBoolean = (parameter: TemplateVersionParameter) => {
  return parameter.type === "bool"
}

export interface ParameterLabelProps {
  parameter: TemplateVersionParameter
}

const ParameterLabel: FC<ParameterLabelProps> = ({ parameter }) => {
  const styles = useStyles()
  const hasDescription = parameter.description && parameter.description !== ""
  const displayName = parameter.display_name
    ? parameter.display_name
    : parameter.name

  return (
    <label htmlFor={parameter.name}>
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
          <Stack spacing={0}>
            <span className={styles.labelPrimary}>{displayName}</span>
            <MemoizedMarkdown className={styles.labelCaption}>
              {parameter.description}
            </MemoizedMarkdown>
          </Stack>
        ) : (
          <span className={styles.labelPrimary}>{displayName}</span>
        )}
      </Stack>
    </label>
  )
}

type Size = "medium" | "small"

export type RichParameterInputProps = Omit<
  TextFieldProps,
  "size" | "onChange"
> & {
  parameter: TemplateVersionParameter
  onChange: (value: string) => void
  size?: Size
}

export const RichParameterInput: FC<RichParameterInputProps> = ({
  parameter,
  size = "medium",
  ...fieldProps
}) => {
  return (
    <Stack
      direction="column"
      spacing={size === "small" ? 1.25 : 2}
      className={size}
      data-testid={`parameter-field-${parameter.name}`}
    >
      <ParameterLabel parameter={parameter} />
      <Box sx={{ display: "flex", flexDirection: "column" }}>
        <RichParameterField {...fieldProps} size={size} parameter={parameter} />
      </Box>
    </Stack>
  )
}

const RichParameterField: React.FC<RichParameterInputProps> = ({
  disabled,
  onChange,
  parameter,
  value,
  size,
  ...props
}) => {
  const styles = useStyles()

  if (isBoolean(parameter)) {
    return (
      <RadioGroup
        id={parameter.name}
        data-testid="parameter-field-bool"
        className={styles.radioGroup}
        value={value}
        onChange={(_, value) => onChange(value)}
      >
        <FormControlLabel
          disabled={disabled}
          value="true"
          control={<Radio size="small" />}
          label="True"
        />
        <FormControlLabel
          disabled={disabled}
          value="false"
          control={<Radio size="small" />}
          label="False"
        />
      </RadioGroup>
    )
  }

  if (parameter.options.length > 0) {
    return (
      <RadioGroup
        id={parameter.name}
        data-testid="parameter-field-options"
        className={styles.radioGroup}
        value={value}
        onChange={(_, value) => onChange(value)}
      >
        {parameter.options.map((option) => (
          <FormControlLabel
            disabled={disabled}
            key={option.name}
            value={option.value}
            control={<Radio size="small" />}
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

    if (typeof value !== "string") {
      throw new Error("Expected value to be a string")
    }

    if (value) {
      try {
        values = JSON.parse(value) as string[]
      } catch (e) {
        console.error("Error parsing list(string) parameter", e)
      }
    }

    return (
      <MultiTextField
        id={parameter.name}
        data-testid="parameter-field-list-of-string"
        label={props.label as string}
        values={values}
        onChange={(values) => {
          try {
            const value = JSON.stringify(values)
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
      id={parameter.name}
      data-testid="parameter-field-text"
      className={styles.textField}
      type={parameter.type}
      disabled={disabled}
      required={parameter.required}
      placeholder={parameter.default_value}
      value={value}
      onChange={(event) => {
        onChange(event.target.value)
      }}
    />
  )
}

const useStyles = makeStyles<Theme>((theme) => ({
  label: {
    marginBottom: theme.spacing(0.5),
  },
  labelCaption: {
    fontSize: 14,
    color: theme.palette.text.secondary,

    ".small &": {
      fontSize: 13,
      lineHeight: "140%",
    },
  },
  labelPrimary: {
    fontSize: 16,
    color: theme.palette.text.primary,
    fontWeight: 600,

    "& p": {
      margin: 0,
      lineHeight: "24px", // Keep the same as ParameterInput
    },

    ".small &": {
      fontSize: 14,
    },
  },
  labelImmutable: {
    marginTop: theme.spacing(0.5),
    marginBottom: theme.spacing(0.5),
    color: colors.yellow[7],
  },
  textField: {
    ".small & .MuiInputBase-root": {
      height: 36,
      fontSize: 14,
      borderRadius: 6,
    },
  },
  radioGroup: {
    ".small & .MuiFormControlLabel-label": {
      fontSize: 14,
    },
    ".small & .MuiRadio-root": {
      padding: theme.spacing(0.75, "9px"), // 8px + 1px border
    },
    ".small & .MuiRadio-root svg": {
      width: 16,
      height: 16,
    },
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

    ".small &": {
      display: "none",
    },
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
    maxHeight: 20,
    width: 20,

    ".small &": {
      maxHeight: 16,
      width: 16,
    },
  },
}))
