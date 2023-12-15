import { type Interpolation, type Theme } from "@emotion/react";
import { ReactNode, type FC } from "react";
import type { AuthMethods } from "api/typesGenerated";
import { PasswordSignInForm } from "./PasswordSignInForm";
import { OAuthSignInForm } from "./OAuthSignInForm";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";

export const Language = {
  emailLabel: "Email",
  passwordLabel: "Password",
  emailInvalid: "Please enter a valid email address.",
  emailRequired: "Please enter an email address.",
  passwordSignIn: "Sign In",
  githubSignIn: "GitHub",
  oidcSignIn: "OpenID Connect",
};

const styles = {
  root: {
    width: "100%",
  },
  title: {
    fontSize: 32,
    fontWeight: 400,
    margin: 0,
    marginBottom: 32,
    lineHeight: 1,

    "& strong": {
      fontWeight: 600,
    },
  },
  alert: {
    marginBottom: 32,
  },
  divider: {
    paddingTop: 24,
    paddingBottom: 24,
    display: "flex",
    alignItems: "center",
    gap: 16,
  },
  dividerLine: (theme) => ({
    width: "100%",
    height: 1,
    backgroundColor: theme.palette.divider,
  }),
  dividerLabel: (theme) => ({
    flexShrink: 0,
    color: theme.palette.text.secondary,
    textTransform: "uppercase",
    fontSize: 12,
    letterSpacing: 1,
  }),
  icon: {
    width: 16,
    height: 16,
  },
} satisfies Record<string, Interpolation<Theme>>;

export interface SignInFormProps {
  isSigningIn: boolean;
  redirectTo: string;
  error?: unknown;
  message?: ReactNode;
  authMethods?: AuthMethods;
  onSubmit: (credentials: { email: string; password: string }) => void;
}

export const SignInForm: FC<React.PropsWithChildren<SignInFormProps>> = ({
  authMethods,
  redirectTo,
  isSigningIn,
  error,
  message,
  onSubmit,
}) => {
  const oAuthEnabled = Boolean(
    authMethods?.github.enabled || authMethods?.oidc.enabled,
  );
  const passwordEnabled = authMethods?.password.enabled ?? true;

  return (
    <div css={styles.root}>
      <h1 css={styles.title}>Sign in</h1>

      {Boolean(error) && (
        <div css={styles.alert}>
          <ErrorAlert error={error} />
        </div>
      )}

      {message && (
        <div css={styles.alert}>
          <Alert severity="info">{message}</Alert>
        </div>
      )}

      {oAuthEnabled && (
        <OAuthSignInForm
          isSigningIn={isSigningIn}
          redirectTo={redirectTo}
          authMethods={authMethods}
        />
      )}

      {passwordEnabled && oAuthEnabled && (
        <div css={styles.divider}>
          <div css={styles.dividerLine} />
          <div css={styles.dividerLabel}>Or</div>
          <div css={styles.dividerLine} />
        </div>
      )}

      {passwordEnabled && (
        <PasswordSignInForm
          onSubmit={onSubmit}
          autoFocus={!oAuthEnabled}
          isSigningIn={isSigningIn}
        />
      )}

      {!passwordEnabled && !oAuthEnabled && (
        <Alert severity="error">No authentication methods configured!</Alert>
      )}
    </div>
  );
};
