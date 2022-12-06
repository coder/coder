import { makeStyles } from "@material-ui/core/styles"
import { FullScreenLoader } from "components/Loader/FullScreenLoader"
import { FC } from "react"
import { useLocation } from "react-router-dom"
import { AuthContext } from "xServices/auth/authXService"
import { LoginErrors, SignInForm } from "components/SignInForm/SignInForm"
import { retrieveRedirect } from "util/redirect"
import { CoderIcon } from "components/Icons/CoderIcon"

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
    <div className={styles.root}>
      <div className={styles.container}>
        <CoderIcon fill="white" opacity={1} className={styles.icon} />
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
        <footer className={styles.footer}>
          Copyright © 2022 Coder Technologies, Inc.
        </footer>
      </div>
    </div>
  )
}

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(3),
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    minHeight: "100vh",
    textAlign: "center",
  },

  container: {
    width: "100%",
    maxWidth: 385,
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    gap: theme.spacing(2),
  },

  icon: {
    fontSize: theme.spacing(8),
  },

  footer: {
    fontSize: 12,
    color: theme.palette.text.secondary,
    marginTop: theme.spacing(3),
  },
}))
