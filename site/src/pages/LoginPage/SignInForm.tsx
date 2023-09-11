import { makeStyles } from "@mui/styles";
import { FormikTouched } from "formik";
import { FC, useState } from "react";
import { AuthMethods } from "api/typesGenerated";
import { useTranslation } from "react-i18next";
import { Maybe } from "components/Conditionals/Maybe";
import { PasswordSignInForm } from "./PasswordSignInForm";
import { OAuthSignInForm } from "./OAuthSignInForm";
import { BuiltInAuthFormValues } from "./SignInForm.types";
import Button from "@mui/material/Button";
import EmailIcon from "@mui/icons-material/EmailOutlined";
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
  alert: {
    marginBottom: theme.spacing(4),
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
}));

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
  const styles = useStyles();
  const commonTranslation = useTranslation("common");
  const loginPageTranslation = useTranslation("loginPage");

  return (
    <div className={styles.root}>
      <h1 className={styles.title}>
        {loginPageTranslation.t("signInTo")}{" "}
        <strong>{commonTranslation.t("coder")}</strong>
      </h1>
      <Maybe condition={error !== undefined}>
        <div className={styles.alert}>
          <ErrorAlert error={error} />
        </div>
      </Maybe>
      <Maybe condition={Boolean(info) && info !== "" && error === undefined}>
        <div className={styles.alert}>
          <Alert severity="info">{info}</Alert>
        </div>
      </Maybe>
      <Maybe condition={passwordEnabled && showPasswordAuth}>
        <PasswordSignInForm
          onSubmit={onSubmit}
          initialTouched={initialTouched}
          isSigningIn={isSigningIn}
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
          isSigningIn={isSigningIn}
          redirectTo={redirectTo}
          authMethods={authMethods}
        />
      </Maybe>

      <Maybe condition={!passwordEnabled && !oAuthEnabled}>
        <Alert severity="error">No authentication methods configured!</Alert>
      </Maybe>

      <Maybe condition={passwordEnabled && !showPasswordAuth}>
        <div className={styles.divider}>
          <div className={styles.dividerLine} />
          <div className={styles.dividerLabel}>Or</div>
          <div className={styles.dividerLine} />
        </div>

        <Button
          fullWidth
          size="large"
          onClick={() => setShowPasswordAuth(true)}
          startIcon={<EmailIcon className={styles.icon} />}
        >
          {loginPageTranslation.t("showPassword")}
        </Button>
      </Maybe>
    </div>
  );
};
