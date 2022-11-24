import { makeStyles } from "@material-ui/core/styles"
import { Logo } from "components/Icons/Logo"
import { FullScreenLoader } from "components/Loader/FullScreenLoader"
import { FC } from "react"
import { useLocation } from "react-router-dom"
import { AuthContext } from "xServices/auth/authXService"
import { LoginErrors, SignInForm } from "components/SignInForm/SignInForm"
import { retrieveRedirect } from "util/redirect"

interface LocationState {
  isRedirect: boolean
}

export interface LoginPageViewProps {
  context: AuthContext
  isLoading: boolean
  onSignIn: (credentials: { email: string; password: string }) => void
}

export const LoginPageView: FC<LoginPageViewProps> = ({
  context,
  isLoading,
  onSignIn,
}) => {
  const location = useLocation()
  const redirectTo = retrieveRedirect(location.search)
  const locationState = location.state
    ? (location.state as LocationState)
    : null
  const isRedirected = locationState ? locationState.isRedirect : false
  const { authError, getUserError, checkPermissionsError, getMethodsError } =
    context
  const styles = useStyles()

  return isLoading ? (
    <FullScreenLoader />
  ) : (
    <div className={styles.container}>
      <div className={styles.left}>
        <Logo fill="white" opacity={1} width={110} />

        <div className={styles.formSection}>
          <SignInForm
            authMethods={context.methods}
            redirectTo={redirectTo}
            isLoading={isLoading}
            loginErrors={{
              [LoginErrors.AUTH_ERROR]: authError,
              [LoginErrors.GET_USER_ERROR]: isRedirected ? getUserError : null,
              [LoginErrors.CHECK_PERMISSIONS_ERROR]: checkPermissionsError,
              [LoginErrors.GET_METHODS_ERROR]: getMethodsError,
            }}
            onSubmit={onSignIn}
          />
        </div>

        <footer className={styles.footer}>
          Copyright © 2022 Coder Technologies, Inc.
        </footer>
      </div>

      <div className={styles.right}>
        <div className={styles.tipWrapper}>
          <div className={styles.tipContent}>
            <h2 className={styles.tipTitle}>Scheduling</h2>
            <p>
              Coder automates your cloud cost control by ensuring developer
              resources are only online while used.
            </p>
            <img
              src="/featured/scheduling.webp"
              alt=""
              className={styles.tipImage}
            />
          </div>
        </div>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  container: {
    padding: theme.spacing(5),
    margin: "auto",
    display: "flex",
    height: "100vh",

    [theme.breakpoints.down("md")]: {
      height: "auto",
      minHeight: "100vh",
    },

    [theme.breakpoints.down("sm")]: {
      padding: theme.spacing(4),
    },
  },

  left: {
    flex: 1,
    display: "flex",
    flexDirection: "column",
    gap: theme.spacing(4),
  },

  right: {
    flex: 1,

    [theme.breakpoints.down("md")]: {
      display: "none",
    },
  },

  formSection: {
    flex: 1,
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
  },

  footer: {
    fontSize: 12,
    color: theme.palette.text.secondary,
  },

  tipWrapper: {
    width: "100%",
    height: "100%",
    borderRadius: theme.shape.borderRadius,
    background: theme.palette.background.paper,
    padding: theme.spacing(5),
    display: "flex",
    justifyContent: "center",
    alignItems: "center",
  },

  tipContent: {
    maxWidth: 570,
    textAlign: "center",
    fontSize: 16,
    color: theme.palette.text.secondary,
    lineHeight: "160%",

    "& p": {
      maxWidth: 440,
      margin: "auto",
    },

    "& strong": {
      color: theme.palette.text.primary,
    },
  },

  tipTitle: {
    fontWeight: 400,
    fontSize: 24,
    margin: 0,
    lineHeight: 1,
    marginBottom: theme.spacing(2),
    color: theme.palette.text.primary,
  },

  tipImage: {
    maxWidth: 570,
    marginTop: theme.spacing(4),
  },
}))
