import Button from "@material-ui/core/Button"
import FormHelperText from "@material-ui/core/FormHelperText"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import { FormikContextType, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"
import { AuthMethods } from "../../api/typesGenerated"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { Welcome } from "../Welcome/Welcome"
import { LoadingButton } from "./../LoadingButton/LoadingButton"

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
  methodsErrorMessage: "Unable to fetch auth methods.",
  passwordSignIn: "Sign In",
  githubSignIn: "GitHub",
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
  methodsErrorMessage?: string
  authMethods?: AuthMethods
  onSubmit: ({ email, password }: { email: string; password: string }) => Promise<void>
}

export const SignInForm: React.FC<SignInFormProps> = ({
  authMethods,
  isLoading,
  authErrorMessage,
  methodsErrorMessage,
  onSubmit,
}) => {
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
          type="email"
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
        {methodsErrorMessage && <FormHelperText error>{Language.methodsErrorMessage}</FormHelperText>}
        <div className={styles.submitBtn}>
          <LoadingButton color="primary" loading={isLoading} fullWidth type="submit" variant="contained">
            {isLoading ? "" : Language.passwordSignIn}
          </LoadingButton>
        </div>
      </form>
      {authMethods?.github && (
        <div className={styles.submitBtn}>
          <Link href="/api/v2/users/oauth2/github/callback">
            <Button color="primary" disabled={isLoading} fullWidth type="submit" variant="contained">
              {Language.githubSignIn}
            </Button>
          </Link>
        </div>
      )}
    </>
  )
}
