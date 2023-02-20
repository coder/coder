import { makeStyles } from "@material-ui/core/styles"
import { FormikTouched } from "formik"
import { FC, useState } from "react"
import { AuthMethods } from "../../api/typesGenerated"
import { useTranslation } from "react-i18next"
import { Maybe } from "../Conditionals/Maybe"
import { PasswordSignInForm } from "./PasswordSignInForm"
import { OAuthSignInForm } from "./OAuthSignInForm"
import { BuiltInAuthFormValues } from "./SignInForm.types"
import Button from "@material-ui/core/Button"
import EmailIcon from "@material-ui/icons/EmailOutlined"
import { AlertBanner } from "components/AlertBanner/AlertBanner"

export enum LoginErrors {
  AUTH_ERROR = "authError",
  GET_USER_ERROR = "getUserError",
  CHECK_PERMISSIONS_ERROR = "checkPermissionsError",
  GET_METHODS_ERROR = "getMethodsError",
}

export const Language = {
  emailLabel: "Email",
  passwordLabel: "Password",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  errorMessages: {
    [LoginErrors.AUTH_ERROR]: "Incorrect email or password.",
    [LoginErrors.GET_USER_ERROR]: "Failed to fetch user details.",
    [LoginErrors.CHECK_PERMISSIONS_ERROR]: "Unable to fetch user permissions.",
    [LoginErrors.GET_METHODS_ERROR]: "Unable to fetch auth methods.",
  },
  passwordSignIn: "Sign In",
  githubSignIn: "GitHub",
  oidcSignIn: "OpenID Connect",
}

const useStyles = makeStyles((theme) => ({
  root: {
    width: "100%",
  },
  title: {
    fontSize: theme.spacing(4),
    fontWeight: 400,
    margin: 0,
    marginBottom: theme.spacing(4),
    lineHeight: 1,

    "& strong": {
      fontWeight: 600,
    },
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
  icon: {
    width: theme.spacing(2),
    height: theme.spacing(2),
  },
}))

export interface SignInFormProps {
  isLoading: boolean
  redirectTo: string
  loginErrors: Partial<Record<LoginErrors, Error | unknown>>
  authMethods?: AuthMethods
  onSubmit: (credentials: { email: string; password: string }) => void
  // initialTouched is only used for testing the error state of the form.
  initialTouched?: FormikTouched<BuiltInAuthFormValues>
}

export const SignInForm: FC<React.PropsWithChildren<SignInFormProps>> = ({
  authMethods,
  redirectTo,
  isLoading,
  loginErrors,
  onSubmit,
  initialTouched,
}) => {
  const oAuthEnabled = Boolean(
    authMethods?.github.enabled || authMethods?.oidc.enabled,
  )
  const passwordEnabled = authMethods?.password.enabled ?? true

  // Hide password auth by default if any OAuth method is enabled
  const [showPasswordAuth, setShowPasswordAuth] = useState(!oAuthEnabled)
  const styles = useStyles()

  const commonTranslation = useTranslation("common")
  const loginPageTranslation = useTranslation("loginPage")

  return (
    <div className={styles.root}>
      <h1 className={styles.title}>
        {loginPageTranslation.t("signInTo")}{" "}
        <strong>{commonTranslation.t("coder")}</strong>
      </h1>
      <Maybe condition={passwordEnabled && showPasswordAuth}>
        <PasswordSignInForm
          loginErrors={loginErrors}
          onSubmit={onSubmit}
          initialTouched={initialTouched}
          isLoading={isLoading}
        />
      </Maybe>
      <Maybe condition={passwordEnabled && showPasswordAuth && oAuthEnabled}>
        <div className={styles.divider}>
          <div className={styles.dividerLine} />
          <div className={styles.dividerLabel}>Or</div>
          <div className={styles.dividerLine} />
        </div>
      </Maybe>
      <Maybe condition={oAuthEnabled}>
        <OAuthSignInForm
          isLoading={isLoading}
          redirectTo={redirectTo}
          authMethods={authMethods}
        />
      </Maybe>

      <Maybe condition={!passwordEnabled && !oAuthEnabled}>
        <AlertBanner
          severity="error"
          text="No authentication methods configured!"
        />
      </Maybe>

      <Maybe condition={passwordEnabled && !showPasswordAuth}>
        <div className={styles.divider}>
          <div className={styles.dividerLine} />
          <div className={styles.dividerLabel}>Or</div>
          <div className={styles.dividerLine} />
        </div>

        <Button
          fullWidth
          onClick={() => setShowPasswordAuth(true)}
          variant="contained"
          startIcon={<EmailIcon className={styles.icon} />}
        >
          {loginPageTranslation.t("showPassword")}
        </Button>
      </Maybe>
    </div>
  )
}
