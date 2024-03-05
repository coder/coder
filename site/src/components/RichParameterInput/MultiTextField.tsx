import type { Interpolation, Theme } from "@emotion/react";
import Chip from "@mui/material/Chip";
import FormHelperText from "@mui/material/FormHelperText";
import type { FC } from "react";

export type MultiTextFieldProps = {
  label: string;
  id?: string;
  values: string[];
  onChange: (values: string[]) => void;
};

export const MultiTextField: FC<MultiTextFieldProps> = ({
  label,
  id,
  values,
  onChange,
}) => {
  return (
    <div>
      <label css={styles.root}>
        {values.map((value, index) => (
          <Chip
            key={index}
            label={value}
            size="small"
            onDelete={() => {
              onChange(values.filter((oldValue) => oldValue !== value));
            }}
          />
        ))}
        <input
          id={id}
          aria-label={label}
          css={styles.input}
          onKeyDown={(event) => {
            if (event.key === ",") {
              event.preventDefault();
              const newValue = event.currentTarget.value;
              onChange([...values, newValue]);
              event.currentTarget.value = "";
              return;
            }

            if (event.key === "Backspace" && event.currentTarget.value === "") {
              event.preventDefault();

              if (values.length === 0) {
                return;
              }

              const lastValue = values[values.length - 1];
              onChange(values.slice(0, -1));
              event.currentTarget.value = lastValue;
            }
          }}
          onBlur={(event) => {
            if (event.currentTarget.value !== "") {
              const newValue = event.currentTarget.value;
              onChange([...values, newValue]);
              event.currentTarget.value = "";
            }
          }}
        />
      </label>

      <FormHelperText>{'Type "," to separate the values'}</FormHelperText>
    </div>
  );
};

const styles = {
  root: (theme) => ({
    border: `1px solid ${theme.palette.divider}`,
    borderRadius: 8,
    minHeight: 48, // Chip height + paddings
    padding: "10px 14px",
    fontSize: 16,
    display: "flex",
    flexWrap: "wrap",
    gap: 8,
    position: "relative",
    margin: "8px 0 4px", // Have same margin than TextField

    "&:has(input:focus)": {
      borderColor: theme.palette.primary.main,
      borderWidth: 2,
      // Compensate for the border width
      top: -1,
      left: -1,
    },
  }),

  input: {
    flexGrow: 1,
    fontSize: "inherit",
    padding: 0,
    border: "none",
    background: "none",

    "&:focus": {
      outline: "none",
    },
  },
} satisfies Record<string, Interpolation<Theme>>;
