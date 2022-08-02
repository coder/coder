import IconButton from "@material-ui/core/IconButton"
import InputAdornment from "@material-ui/core/InputAdornment"
import { makeStyles } from "@material-ui/core/styles"
import TextField, { TextFieldProps } from "@material-ui/core/TextField"
import VisibilityOffOutlined from "@material-ui/icons/VisibilityOffOutlined"
import VisibilityOutlined from "@material-ui/icons/VisibilityOutlined"
import React, { useCallback, useState } from "react"

type PasswordFieldProps = Omit<TextFieldProps, "InputProps" | "type">

export const PasswordField: React.FC<React.PropsWithChildren<PasswordFieldProps>> = ({ variant = "outlined", ...rest }) => {
  const styles = useStyles()
  const [showPassword, setShowPassword] = useState<boolean>(false)

  const handleVisibilityChange = useCallback(
    () => setShowPassword((showPassword) => !showPassword),
    [],
  )
  const VisibilityIcon = showPassword ? VisibilityOffOutlined : VisibilityOutlined

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
