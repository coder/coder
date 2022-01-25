import { makeStyles } from "@material-ui/core/styles"
import { FormikContextType, useFormik } from "formik"
import { useRouter } from "next/router"
import React from "react"
import { useSWRConfig } from "swr"
import * as Yup from "yup"

import { Welcome } from "./Welcome"
import { FormTextField } from "../Form"
import * as API from "./../../api"
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
  loginTypeToggleWrapper: {
    marginTop: theme.spacing(2),
    display: "flex",
    justifyContent: "center",
  },
  loginTypeToggleBtn: {
    color: theme.palette.text.primary,
    // We want opacity so that this isn't super highlighted for the user.
    // In most cases, they shouldn't want to switch login types.
    opacity: 0.5,
    "&:hover": {
      cursor: "pointer",
      opacity: 1,
      textDecoration: "underline",
    },
  },
  loginTypeToggleBtnFocusVisible: {
    opacity: 1,
    textDecoration: "underline",
  },
  loginTypeBtn: {
    backgroundColor: "#2A2B45",
    textTransform: "none",

    "&:not(:first-child)": {
      marginTop: theme.spacing(2),
    },
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
        await router.push("/")
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
            eventTransform={(email: string) => email.trim()}
            form={form}
            formFieldName="email"
            fullWidth
            inputProps={{
              id: "signin-form-inpt-email",
            }}
            margin="none"
            placeholder="Email"
            variant="outlined"
          />
          <FormTextField
            autoComplete="current-password"
            form={form}
            formFieldName="password"
            fullWidth
            inputProps={{
              id: "signin-form-inpt-password",
            }}
            isPassword
            margin="none"
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
