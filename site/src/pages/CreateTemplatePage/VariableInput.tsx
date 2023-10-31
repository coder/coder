import { type Interpolation, type Theme } from "@emotion/react";
import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import TextField from "@mui/material/TextField";
import { Stack } from "components/Stack/Stack";
import { type FC } from "react";
import type { TemplateVersionVariable } from "api/typesGenerated";

const isBoolean = (variable: TemplateVersionVariable) => {
  return variable.type === "bool";
};

const VariableLabel: FC<{ variable: TemplateVersionVariable }> = ({
  variable,
}) => {
  return (
    <label htmlFor={variable.name}>
      <span css={styles.labelName}>
        var.{variable.name}
        {!variable.required && " (optional)"}
      </span>
      <span css={styles.labelDescription}>{variable.description}</span>
    </label>
  );
};

export interface VariableInputProps {
  disabled?: boolean;
  variable: TemplateVersionVariable;
  onChange: (value: string) => void;
  defaultValue?: string;
}

export const VariableInput: FC<VariableInputProps> = ({
  disabled,
  onChange,
  variable,
  defaultValue,
}) => {
  return (
    <Stack direction="column" spacing={0.75}>
      <VariableLabel variable={variable} />
      <div css={styles.input}>
        <VariableField
          disabled={disabled}
          onChange={onChange}
          variable={variable}
          defaultValue={defaultValue}
        />
      </div>
    </Stack>
  );
};

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
          onChange(event.target.value);
        }}
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

  return (
    <TextField
      autoComplete="off"
      id={variable.name}
      size="small"
      disabled={disabled}
      placeholder={variable.sensitive ? "" : variable.default_value}
      required={variable.required}
      defaultValue={
        variable.sensitive ? "" : defaultValue ?? variable.default_value
      }
      onChange={(event) => {
        onChange(event.target.value);
      }}
      type={
        variable.type === "number"
          ? "number"
          : variable.sensitive
          ? "password"
          : "string"
      }
    />
  );
};

const styles = {
  labelName: (theme) => ({
    fontSize: 14,
    color: theme.palette.text.secondary,
    display: "block",
    marginBottom: theme.spacing(0.5),
  }),
  labelDescription: (theme) => ({
    fontSize: 16,
    color: theme.palette.text.primary,
    display: "block",
    fontWeight: 600,
  }),
  input: {
    display: "flex",
    flexDirection: "column",
  },
} satisfies Record<string, Interpolation<Theme>>;
