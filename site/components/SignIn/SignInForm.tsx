import { makeStyles } from "@material-ui/core/styles"
import { FormikContextType, useFormik } from "formik"
import { NextRouter, useRouter } from "next/router"
import React from "react"
import { useSWRConfig } from "swr"
import * as Yup from "yup"

import { Welcome } from "./Welcome"
import { FormTextField } from "../Form"
import * as API from "./../../api"
import { LoadingButton } from "./../Button"
import { firstOrItem } from "../../util/array"

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

export interface SignInProps {
  loginHandler?: (email: string, password: string) => Promise<void>
}

export const SignInForm: React.FC<SignInProps> = ({
  loginHandler = (email: string, password: string) => API.login(email, password),
}) => {
  const router = useRouter()
  const styles = useStyles()
  const { mutate } = useSWRConfig()

  const form: FormikContextType<BuiltInAuthFormValues> = useFormik<BuiltInAuthFormValues>({
    initialValues: {
      email: "",
      password: "",
    },
    validationSchema,
    onSubmit: async ({ email, password }, helpers) => {
      try {
        await loginHandler(email, password)
        // Tell SWR to invalidate the cache for the user endpoint
        await mutate("/api/v2/users/me")

        const redirect = getRedirectFromRouter(router)
        await router.push(redirect)
      } catch (err) {
        helpers.setFieldError("password", "The username or password is incorrect.")
      }
    },
  })

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
          />
        </div>
        <div className={styles.submitBtn}>
          <LoadingButton
            color="primary"
            loading={form.isSubmitting}
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

const getRedirectFromRouter = (router: NextRouter) => {
  const defaultRedirect = "/"
  if (router.query.redirect) {
    return firstOrItem(router.query.redirect, defaultRedirect)
  } else {
    return defaultRedirect
  }
}
