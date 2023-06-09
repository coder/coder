import IconButton from "@mui/material/IconButton"
import InputAdornment from "@mui/material/InputAdornment"
import { makeStyles } from "@mui/styles"
import TextField, { TextFieldProps } from "@mui/material/TextField"
import VisibilityOffOutlined from "@mui/icons-material/VisibilityOffOutlined"
import VisibilityOutlined from "@mui/icons-material/VisibilityOutlined"
import { useCallback, useState, FC, PropsWithChildren } from "react"

type PasswordFieldProps = Omit<TextFieldProps, "InputProps" | "type">

export const PasswordField: FC<PropsWithChildren<PasswordFieldProps>> = ({
  variant = "outlined",
  ...rest
}) => {
  const styles = useStyles()
  const [showPassword, setShowPassword] = useState<boolean>(false)

  const handleVisibilityChange = useCallback(
    () => setShowPassword((showPassword) => !showPassword),
    [],
  )
  const VisibilityIcon = showPassword
    ? VisibilityOffOutlined
    : VisibilityOutlined

  return (
    <TextField
      {...rest}
      type={showPassword ? "text" : "password"}
      variant={variant}
      InputProps={{
        endAdornment: (
          <InputAdornment position="end">
            <IconButton
              aria-label="toggle password visibility"
              onClick={handleVisibilityChange}
              size="small"
            >
              <VisibilityIcon className={styles.visibilityIcon} />
            </IconButton>
          </InputAdornment>
        ),
      }}
    />
  )
}

const useStyles = makeStyles({
  visibilityIcon: {
    fontSize: 20,
  },
})
