import { makeStyles } from "@material-ui/core/styles"
import { FormikContextType, useFormik } from "formik"
import React from "react"
import * as Yup from "yup"

import { Welcome } from "./Welcome"
import { FormTextField } from "../Form"
import FormHelperText from "@material-ui/core/FormHelperText"
import { LoadingButton } from "./../Button"

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
        <div>
          <FormTextField
            label="Email"
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
            variant="outlined"
          />
          <FormTextField
            label="Password"
            autoComplete="current-password"
            className={styles.loginTextField}
            form={form}
            formFieldName="password"
            fullWidth
            inputProps={{
              id: "signin-form-inpt-password",
            }}
            isPassword
            variant="outlined"
          />
          {authErrorMessage && (
            <FormHelperText data-testid="sign-in-error" error>
              {authErrorMessage}
            </FormHelperText>
          )}
        </div>
        <div className={styles.submitBtn}>
          <LoadingButton
            color="primary"
            loading={isLoading}
            fullWidth
            id="signin-form-submit"
            type="submit"
            variant="contained"
          >
            {isLoading ? "" : "Sign In"}
          </LoadingButton>
        </div>
      </form>
    </>
  )
}
