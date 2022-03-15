import { makeStyles } from "@material-ui/core/styles"
import { FormikContextType, useFormik } from "formik"
import { Location } from "history"
import { useLocation, Navigate } from "react-router-dom"
import React from "react"
import * as Yup from "yup"

import { Welcome } from "./Welcome"
import { FormTextField } from "../Form"
import { LoadingButton } from "./../Button"
import { userXService } from "../../xServices/user/userXService"
import { useActor } from "@xstate/react"

/**
 * BuiltInAuthFormValues describes a form using built-in (email/password)
 * authentication. This form may not always be present depending on external
 * auth providers available and administrative configurations
 */
interface BuiltInAuthFormValues {
  email: string
  password: string
}

const validationSchema = Yup.object({
  email: Yup.string().required("Email is required."),
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

export const SignInForm: React.FC = () => {
  const location = useLocation()
  const styles = useStyles()
  const [userState, userSend] = useActor(userXService)
  const { authError } = userState.context

  const form: FormikContextType<BuiltInAuthFormValues> = useFormik<BuiltInAuthFormValues>({
    initialValues: {
      email: "",
      password: "",
    },
    validationSchema,
    onSubmit: async ({ email, password }) => {
      userSend({ type: 'SIGN_IN', email, password })
    },
  })

  if (userState.matches('signedIn')) {
    return <Navigate to={getRedirectFromLocation(location)} />
  }

  return (
    <>
      <Welcome />
      <form onSubmit={form.handleSubmit}>
        <div>
          <FormTextField
            autoComplete="email"
            autoFocus
            className={styles.loginTextField}
            eventTransform={(email: string) => email.trim()}
            form={form}
            formFieldName="email"
            fullWidth
            inputProps={{
              id: "signin-form-inpt-email",
            }}
            placeholder="Email"
            variant="outlined"
          />
          <FormTextField
            autoComplete="current-password"
            className={styles.loginTextField}
            form={form}
            formFieldName="password"
            fullWidth
            inputProps={{
              id: "signin-form-inpt-password",
            }}
            isPassword
            placeholder="Password"
            variant="outlined"
            helperText={authError ? (authError as Error).message : ''}
          />
        </div>
        <div className={styles.submitBtn}>
          <LoadingButton
            color="primary"
            loading={userState.hasTag('loading')}
            fullWidth
            id="signin-form-submit"
            type="submit"
            variant="contained"
          >
            Sign In
          </LoadingButton>
        </div>
      </form>
    </>
  )
}

const getRedirectFromLocation = (location: Location) => {
  const defaultRedirect = "/"

  const searchParams = new URLSearchParams(location.search)
  const redirect = searchParams.get("redirect")
  return redirect ? redirect : defaultRedirect
}
