import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import TextField from "@material-ui/core/TextField"
import GitHubIcon from "@material-ui/icons/GitHub"
import { ErrorSummary } from "components/ErrorSummary/ErrorSummary"
import { Stack } from "components/Stack/Stack"
import { FormikContextType, FormikTouched, useFormik } from "formik"
import { FC } from "react"
import * as Yup from "yup"
import { AuthMethods } from "../../api/typesGenerated"
import { getFormHelpersWithError, onChangeTrimmed } from "../../util/formUtils"
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
  buttonIcon: {
    width: 14,
    height: 14,
  },
  divider: {
    paddingTop: theme.spacing(3),
    paddingBottom: theme.spacing(3),
    display: "flex",
    alignItems: "center",
    gap: theme.spacing(2),
  },
  dividerLine: {
    width: "100%",
    height: 1,
    backgroundColor: theme.palette.divider,
  },
  dividerLabel: {
    flexShrink: 0,
    color: theme.palette.text.secondary,
    textTransform: "uppercase",
    fontSize: 12,
    letterSpacing: 1,
  },
}))

export interface SignInFormProps {
  isLoading: boolean
  redirectTo: string
  authError?: Error | unknown
  methodsError?: Error | unknown
  authMethods?: AuthMethods
  onSubmit: ({ email, password }: { email: string; password: string }) => Promise<void>
  // initialTouched is only used for testing the error state of the form.
  initialTouched?: FormikTouched<BuiltInAuthFormValues>
}

export const SignInForm: FC<SignInFormProps> = ({
  authMethods,
  redirectTo,
  isLoading,
  authError,
  methodsError,
  onSubmit,
  initialTouched,
}) => {
  const styles = useStyles()

  const form: FormikContextType<BuiltInAuthFormValues> = useFormik<BuiltInAuthFormValues>({
    initialValues: {
      email: "",
      password: "",
    },
    validationSchema,
    // The email field has an autoFocus, but users may login with a button click.
    // This is set to `false` in order to keep the autoFocus, validateOnChange
    // and Formik experience friendly. Validation will kick in onChange (any
    // field), or after a submission attempt.
    validateOnBlur: false,
    onSubmit,
    initialTouched,
  })
  const getFieldHelpers = getFormHelpersWithError<BuiltInAuthFormValues>(form, authError)

  return (
    <>
      <Welcome />
      <form onSubmit={form.handleSubmit}>
        <Stack>
          {authError && (
            <ErrorSummary error={authError} defaultMessage={Language.authErrorMessage} />
          )}
          {methodsError && (
            <ErrorSummary error={methodsError} defaultMessage={Language.methodsErrorMessage} />
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
            <LoadingButton loading={isLoading} fullWidth type="submit" variant="contained">
              {isLoading ? "" : Language.passwordSignIn}
            </LoadingButton>
          </div>
        </Stack>
      </form>
      {authMethods?.github && (
        <>
          <div className={styles.divider}>
            <div className={styles.dividerLine} />
            <div className={styles.dividerLabel}>Or</div>
            <div className={styles.dividerLine} />
          </div>

          <div>
            <Link
              underline="none"
              href={`/api/v2/users/oauth2/github/callback?redirect=${encodeURIComponent(
                redirectTo,
              )}`}
            >
              <Button
                startIcon={<GitHubIcon className={styles.buttonIcon} />}
                disabled={isLoading}
                fullWidth
                type="submit"
                variant="contained"
              >
                {Language.githubSignIn}
              </Button>
            </Link>
          </div>
        </>
      )}
    </>
  )
}
