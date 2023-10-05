import Box from "@mui/material/Box";
import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import TextField, { TextFieldProps } from "@mui/material/TextField";
import Tooltip from "@mui/material/Tooltip";
import { type Interpolation, type Theme, useTheme } from "@emotion/react";
import { type FC } from "react";
import { TemplateVersionParameter } from "api/typesGenerated";
import { MemoizedMarkdown } from "components/Markdown/Markdown";
import { Stack } from "components/Stack/Stack";
import { colors } from "theme/colors";
import { MultiTextField } from "./MultiTextField";

const isBoolean = (parameter: TemplateVersionParameter) => {
  return parameter.type === "bool";
};

const styles = {
  label: (theme) => ({
    marginBottom: theme.spacing(0.5),
  }),
  labelCaption: (theme) => ({
    fontSize: 14,
    color: theme.palette.text.secondary,

    ".small &": {
      fontSize: 13,
      lineHeight: "140%",
    },
  }),
  labelPrimary: (theme) => ({
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
  }),
  labelImmutable: (theme) => ({
    marginTop: theme.spacing(0.5),
    marginBottom: theme.spacing(0.5),
    color: colors.yellow[7],
  }),
  textField: {
    ".small & .MuiInputBase-root": {
      height: 36,
      fontSize: 14,
      borderRadius: 6,
    },
  },
  radioGroup: (theme) => ({
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
  }),
  checkbox: (theme) => ({
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(1),
  }),
  labelIconWrapper: (theme) => ({
    width: theme.spacing(2.5),
    height: theme.spacing(2.5),
    display: "block",

    ".small &": {
      display: "none",
    },
  }),
  labelIcon: {
    width: "100%",
    height: "100%",
    objectFit: "contain",
  },
  optionIcon: {
    pointerEvents: "none",
    maxHeight: 20,
    width: 20,

    ".small &": {
      maxHeight: 16,
      width: 16,
    },
  },
} satisfies Record<string, Interpolation<Theme>>;

export interface ParameterLabelProps {
  parameter: TemplateVersionParameter;
}

const ParameterLabel: FC<ParameterLabelProps> = ({ parameter }) => {
  const hasDescription = parameter.description && parameter.description !== "";
  const displayName = parameter.display_name
    ? parameter.display_name
    : parameter.name;

  return (
    <label htmlFor={parameter.name}>
      <Stack direction="row" alignItems="center">
        {parameter.icon && (
          <span css={styles.labelIconWrapper}>
            <img
              css={styles.labelIcon}
              alt="Parameter icon"
              src={parameter.icon}
            />
          </span>
        )}

        {hasDescription ? (
          <Stack spacing={0}>
            <span css={styles.labelPrimary}>{displayName}</span>
            <MemoizedMarkdown css={styles.labelCaption}>
              {parameter.description}
            </MemoizedMarkdown>
          </Stack>
        ) : (
          <span css={styles.labelPrimary}>{displayName}</span>
        )}
      </Stack>
    </label>
  );
};

type Size = "medium" | "small";

export type RichParameterInputProps = Omit<
  TextFieldProps,
  "size" | "onChange"
> & {
  parameter: TemplateVersionParameter;
  onChange: (value: string) => void;
  size?: Size;
};

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
  );
};

const RichParameterField: React.FC<RichParameterInputProps> = ({
  disabled,
  onChange,
  parameter,
  value,
  size,
  ...props
}) => {
  const theme = useTheme();
  const small = size === "small";

  if (isBoolean(parameter)) {
    return (
      <RadioGroup
        id={parameter.name}
        data-testid="parameter-field-bool"
        css={styles.radioGroup}
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
    );
  }

  if (parameter.options.length > 0) {
    return (
      <RadioGroup
        id={parameter.name}
        data-testid="parameter-field-options"
        css={styles.radioGroup}
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
              <Stack direction="row" alignItems="center">
                {option.icon && (
                  <img
                    css={styles.optionIcon}
                    src={option.icon}
                    alt="Parameter icon"
                  />
                )}
                {option.description ? (
                  <Stack
                    spacing={small ? 1 : 0}
                    alignItems={small ? "center" : undefined}
                    direction={small ? "row" : "column"}
                    css={{
                      padding: small ? undefined : `${theme.spacing(0.5)} 0`,
                    }}
                  >
                    {small ? (
                      <Tooltip
                        title={
                          <MemoizedMarkdown>
                            {option.description}
                          </MemoizedMarkdown>
                        }
                      >
                        <Box>{option.name}</Box>
                      </Tooltip>
                    ) : (
                      <>
                        <span>{option.name}</span>
                        <MemoizedMarkdown css={styles.labelCaption}>
                          {option.description}
                        </MemoizedMarkdown>
                      </>
                    )}
                  </Stack>
                ) : (
                  option.name
                )}
              </Stack>
            }
          />
        ))}
      </RadioGroup>
    );
  }

  if (parameter.type === "list(string)") {
    let values: string[] = [];

    if (typeof value !== "string") {
      throw new Error("Expected value to be a string");
    }

    if (value) {
      try {
        values = JSON.parse(value) as string[];
      } catch (e) {
        console.error("Error parsing list(string) parameter", e);
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
            const value = JSON.stringify(values);
            onChange(value);
          } catch (e) {
            console.error("Error on change of list(string) parameter", e);
          }
        }}
      />
    );
  }

  // A text field can technically handle all cases!
  // As other cases become more prominent (like filtering for numbers),
  // we should break this out into more finely scoped input fields.
  return (
    <TextField
      {...props}
      id={parameter.name}
      data-testid="parameter-field-text"
      css={styles.textField}
      type={parameter.type}
      disabled={disabled}
      required={parameter.required}
      placeholder={parameter.default_value}
      value={value}
      onChange={(event) => {
        onChange(event.target.value);
      }}
    />
  );
};
