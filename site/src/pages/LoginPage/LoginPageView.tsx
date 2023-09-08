import { makeStyles } from "@mui/styles";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";
import { FC } from "react";
import { useLocation } from "react-router-dom";
import { AuthContext, UnauthenticatedData } from "xServices/auth/authXService";
import { SignInForm } from "pages/LoginPage/SignInForm/SignInForm";
import { retrieveRedirect } from "utils/redirect";
import { CoderIcon } from "components/Icons/CoderIcon";

export interface LoginPageViewProps {
  context: AuthContext;
  isLoading: boolean;
  isSigningIn: boolean;
  onSignIn: (credentials: { email: string; password: string }) => void;
}

export const LoginPageView: FC<LoginPageViewProps> = ({
  context,
  isLoading,
  isSigningIn,
  onSignIn,
}) => {
  const location = useLocation();
  const redirectTo = retrieveRedirect(location.search);
  const { error } = context;
  const data = context.data as UnauthenticatedData;
  const styles = useStyles();
  // This allows messages to be displayed at the top of the sign in form.
  // Helpful for any redirects that want to inform the user of something.
  const info = new URLSearchParams(location.search).get("info") || undefined;

  return isLoading ? (
    <FullScreenLoader />
  ) : (
    <div className={styles.root}>
      <div className={styles.container}>
        <CoderIcon fill="white" opacity={1} className={styles.icon} />
        <SignInForm
          authMethods={data.authMethods}
          redirectTo={redirectTo}
          isSigningIn={isSigningIn}
          error={error}
          info={info}
          onSubmit={onSignIn}
        />
        <footer className={styles.footer}>
          Copyright © {new Date().getFullYear()} Coder Technologies, Inc.
        </footer>
      </div>
    </div>
  );
};

const useStyles = makeStyles((theme) => ({
  root: {
    padding: theme.spacing(3),
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    minHeight: "100%",
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
}));
