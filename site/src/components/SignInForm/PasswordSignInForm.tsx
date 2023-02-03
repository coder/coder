import { Stack } from "../Stack/Stack"
import { AlertBanner } from "../AlertBanner/AlertBanner"
import TextField from "@material-ui/core/TextField"
import { getFormHelpers, onChangeTrimmed } from "../../util/formUtils"
import { LoadingButton } from "../LoadingButton/LoadingButton"
import { Language, LoginErrors } from "./SignInForm"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import * as Yup from "yup"
import { FC } from "react"
import { BuiltInAuthFormValues } from "./SignInForm.types"

type PasswordSignInFormProps = {
  loginErrors: Partial<Record<LoginErrors, Error | unknown>>
  onSubmit: (credentials: { email: string; password: string }) => void
  initialTouched?: FormikTouched<BuiltInAuthFormValues>
  isLoading: boolean
}

export const PasswordSignInForm: FC<PasswordSignInFormProps> = ({
  loginErrors,
  onSubmit,
  initialTouched,
  isLoading,
}) => {
  const validationSchema = Yup.object({
    email: Yup.string()
      .trim()
      .email(Language.emailInvalid)
      .required(Language.emailRequired),
    password: Yup.string(),
  })

  const form: FormikContextType<BuiltInAuthFormValues> =
    useFormik<BuiltInAuthFormValues>({
      initialValues: {
        email: "",
        password: "",
      },
      validationSchema,
      // The email field has an autoFocus, but users may log in with a button click.
      // This is set to `false` in order to keep the autoFocus, validateOnChange
      // and Formik experience friendly. Validation will kick in onChange (any
      // field), or after a submission attempt.
      validateOnBlur: false,
      onSubmit,
      initialTouched,
    })
  const getFieldHelpers = getFormHelpers<BuiltInAuthFormValues>(
    form,
    loginErrors.authError,
  )

  return (
    <form onSubmit={form.handleSubmit}>
      <Stack>
        {Object.keys(loginErrors).map(
          (errorKey: string) =>
            Boolean(loginErrors[errorKey as LoginErrors]) && (
              <AlertBanner
                key={errorKey}
                severity="error"
                error={loginErrors[errorKey as LoginErrors]}
                text={Language.errorMessages[errorKey as LoginErrors]}
              />
            ),
        )}
        <TextField
          {...getFieldHelpers("email")}
          onChange={onChangeTrimmed(form)}
          autoFocus
          autoComplete="email"
          fullWidth
          label={Language.emailLabel}
          type="email"
          variant="outlined"
        />
        <TextField
          {...getFieldHelpers("password")}
          autoComplete="current-password"
          fullWidth
          id="password"
          label={Language.passwordLabel}
          type="password"
          variant="outlined"
        />
        <div>
          <LoadingButton
            loading={isLoading}
            fullWidth
            type="submit"
            variant="contained"
          >
            {isLoading ? "" : Language.passwordSignIn}
          </LoadingButton>
        </div>
      </Stack>
    </form>
  )
}
