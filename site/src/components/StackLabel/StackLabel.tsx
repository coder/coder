import FormHelperText, {
  type FormHelperTextProps,
} from "@mui/material/FormHelperText";
import type { ComponentProps, FC } from "react";
import { Stack } from "components/Stack/Stack";

/**
 * Use these components as the label in FormControlLabel when implementing radio
 * buttons, checkboxes, or switches to ensure proper styling.
 */

export const StackLabel: FC<ComponentProps<typeof Stack>> = (props) => {
  return (
    <Stack
      spacing={0.5}
      css={{ paddingLeft: 12, fontWeight: 500 }}
      {...props}
    />
  );
};

export const StackLabelHelperText: FC<FormHelperTextProps> = (props) => {
  return (
    <FormHelperText
      css={(theme) => ({
        marginTop: 0,

        "& strong": {
          color: theme.palette.text.primary,
        },
      })}
      {...props}
    />
  );
};
