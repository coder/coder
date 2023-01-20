import { TextFieldProps } from "@material-ui/core/TextField"

export type IconFieldProps = TextFieldProps & {
  onPickEmoji: (value: string) => void
}
