import { type Interpolation, type Theme } from "@emotion/react";
import { type FormikTouched } from "formik";
import { type FC, useState } from "react";
import type { AuthMethods } from "api/typesGenerated";
import { PasswordSignInForm } from "./PasswordSignInForm";
import { OAuthSignInForm } from "./OAuthSignInForm";
import { type BuiltInAuthFormValues } from "./SignInForm.types";
import Button from "@mui/material/Button";
import EmailIcon from "@mui/icons-material/EmailOutlined";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { getApplicationName } from "utils/appearance";

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
  info?: string;
  authMethods?: AuthMethods;
  onSubmit: (credentials: { email: string; password: string }) => void;
  // initialTouched is only used for testing the error state of the form.
  initialTouched?: FormikTouched<BuiltInAuthFormValues>;
}

export const SignInForm: FC<React.PropsWithChildren<SignInFormProps>> = ({
  authMethods,
  redirectTo,
  isSigningIn,
  error,
  info,
  onSubmit,
  initialTouched,
}) => {
  const oAuthEnabled = Boolean(
    authMethods?.github.enabled || authMethods?.oidc.enabled,
  );
  const passwordEnabled = authMethods?.password.enabled ?? true;
  // Hide password auth by default if any OAuth method is enabled
  const [showPasswordAuth, setShowPasswordAuth] = useState(!oAuthEnabled);
  const applicationName = getApplicationName();

  return (
    <div css={styles.root}>
      <h1 css={styles.title}>
        Sign in to <strong>{applicationName}</strong>
      </h1>

      {Boolean(error) && (
        <div css={styles.alert}>
          <ErrorAlert error={error} />
        </div>
      )}

      {Boolean(info) && Boolean(error) && (
        <div css={styles.alert}>
          <Alert severity="info">{info}</Alert>
        </div>
      )}

      {passwordEnabled && showPasswordAuth && (
        <PasswordSignInForm
          onSubmit={onSubmit}
          initialTouched={initialTouched}
          isSigningIn={isSigningIn}
        />
      )}

      {passwordEnabled && showPasswordAuth && oAuthEnabled && (
        <div css={styles.divider}>
          <div css={styles.dividerLine} />
          <div css={styles.dividerLabel}>Or</div>
          <div css={styles.dividerLine} />
        </div>
      )}

      {oAuthEnabled && (
        <OAuthSignInForm
          isSigningIn={isSigningIn}
          redirectTo={redirectTo}
          authMethods={authMethods}
        />
      )}

      {!passwordEnabled && !oAuthEnabled && (
        <Alert severity="error">No authentication methods configured!</Alert>
      )}

      {passwordEnabled && !showPasswordAuth && (
        <>
          <div css={styles.divider}>
            <div css={styles.dividerLine} />
            <div css={styles.dividerLabel}>Or</div>
            <div css={styles.dividerLine} />
          </div>

          <Button
            fullWidth
            size="large"
            onClick={() => setShowPasswordAuth(true)}
            startIcon={<EmailIcon css={styles.icon} />}
          >
            Email and password
          </Button>
        </>
      )}
    </div>
  );
};
