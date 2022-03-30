import { makeStyles } from "@material-ui/core/styles"
import { FormikContextType, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"

import { Welcome } from "./Welcome"
import FormHelperText from "@material-ui/core/FormHelperText"
import { LoadingButton } from "./../Button"
import TextField from "@material-ui/core/TextField"
import { getFormHelpers, onChangeTrimmed } from "../Form"

/**
 * BuiltInAuthFormValues describes a form using built-in (email/password)
 * authentication. This form may not always be present depending on external
 * auth providers available and administrative configurations
 */
interface BuiltInAuthFormValues {
  email: string
  password: string
}

export const Language = {
  emailLabel: "Email",
  passwordLabel: "Password",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  authErrorMessage: "Incorrect email or password.",
  signIn: "Sign In",
}

const validationSchema = Yup.object({
  email: Yup.string().trim().email(Language.emailInvalid).required(Language.emailRequired),
  password: Yup.string(),
})

const useStyles = makeStyles((theme) => ({
  loginBtnWrapper: {
    marginTop: theme.spacing(6),
    borderTop: `1px solid ${theme.palette.action.disabled}`,
    paddingTop: theme.spacing(3),
  },
  loginTextField: {
    marginTop: theme.spacing(2),
  },
  submitBtn: {
    marginTop: theme.spacing(2),
  },
}))

export interface SignInFormProps {
  isLoading: boolean
  authErrorMessage?: string
  onSubmit: ({ email, password }: { email: string; password: string }) => Promise<void>
}

export const SignInForm: React.FC<SignInFormProps> = ({ isLoading, authErrorMessage, onSubmit }) => {
  const styles = useStyles()

  const form: FormikContextType<BuiltInAuthFormValues> = useFormik<BuiltInAuthFormValues>({
    initialValues: {
      email: "",
      password: "",
    },
    validationSchema,
    onSubmit,
  })

  return (
    <>
      <Welcome />
      <form onSubmit={form.handleSubmit}>
        <TextField
          {...getFormHelpers<BuiltInAuthFormValues>(form, "email")}
          onChange={onChangeTrimmed(form)}
          autoFocus
          autoComplete="email"
          className={styles.loginTextField}
          fullWidth
          label={Language.emailLabel}
          variant="outlined"
        />
        <TextField
          {...getFormHelpers<BuiltInAuthFormValues>(form, "password")}
          autoComplete="current-password"
          className={styles.loginTextField}
          fullWidth
          id="password"
          label={Language.passwordLabel}
          type="password"
          variant="outlined"
        />
        {authErrorMessage && <FormHelperText error>{Language.authErrorMessage}</FormHelperText>}
        <div className={styles.submitBtn}>
          <LoadingButton color="primary" loading={isLoading} fullWidth type="submit" variant="contained">
            {isLoading ? "" : Language.signIn}
          </LoadingButton>
        </div>
      </form>
    </>
  )
}
