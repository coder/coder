import { TextFieldProps } from "@mui/material/TextField";

export type IconFieldProps = TextFieldProps & {
  onPickEmoji: (value: string) => void;
};
