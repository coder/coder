import FormControlLabel from "@mui/material/FormControlLabel";
import Radio from "@mui/material/Radio";
import RadioGroup from "@mui/material/RadioGroup";
import TextField from "@mui/material/TextField";
import { TemplateVersionVariable } from "api/typesGenerated";
import { FC, useState } from "react";

export const SensitiveVariableHelperText = () => {
  return (
    <span>
      This variable is sensitive. The previous value will be used if empty.
    </span>
  );
};

export interface TemplateVariableFieldProps {
  templateVersionVariable: TemplateVersionVariable;
  initialValue: string;
  disabled: boolean;
  onChange: (value: string) => void;
}

export const TemplateVariableField: FC<TemplateVariableFieldProps> = ({
  templateVersionVariable,
  initialValue,
  disabled,
  onChange,
  ...props
}) => {
  const [variableValue, setVariableValue] = useState(initialValue);
  if (isBoolean(templateVersionVariable)) {
    return (
      <RadioGroup
        defaultValue={variableValue}
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
      {...props}
      type={
        templateVersionVariable.type === "number"
          ? "number"
          : templateVersionVariable.sensitive
            ? "password"
            : "string"
      }
      disabled={disabled}
      autoFocus
      fullWidth
      label={templateVersionVariable.name}
      value={variableValue}
      placeholder={
        templateVersionVariable.sensitive
          ? ""
          : templateVersionVariable.default_value
      }
      onChange={(event) => {
        setVariableValue(event.target.value);
        onChange(event.target.value);
      }}
    />
  );
};

const isBoolean = (variable: TemplateVersionVariable) => {
  return variable.type === "bool";
};
