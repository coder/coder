import Link from "@mui/material/Link";
import Button from "@mui/material/Button";
import GitHubIcon from "@mui/icons-material/GitHub";
import KeyIcon from "@mui/icons-material/VpnKey";
import Box from "@mui/material/Box";
import { type FC } from "react";
import { useTheme } from "@emotion/react";
import { Language } from "./SignInForm";
import { type AuthMethods } from "api/typesGenerated";

type OAuthSignInFormProps = {
  isSigningIn: boolean;
  redirectTo: string;
  authMethods?: AuthMethods;
};

export const OAuthSignInForm: FC<OAuthSignInFormProps> = ({
  isSigningIn,
  redirectTo,
  authMethods,
}) => {
  const theme = useTheme();

  const iconStyles = {
    width: theme.spacing(2),
    height: theme.spacing(2),
  };

  return (
    <Box display="grid" gap="16px">
      {authMethods?.github.enabled && (
        <Link
          href={`/api/v2/users/oauth2/github/callback?redirect=${encodeURIComponent(
            redirectTo,
          )}`}
        >
          <Button
            startIcon={<GitHubIcon css={iconStyles} />}
            disabled={isSigningIn}
            fullWidth
            type="submit"
            size="large"
          >
            {Language.githubSignIn}
          </Button>
        </Link>
      )}

      {authMethods?.oidc.enabled && (
        <Link
          href={`/api/v2/users/oidc/callback?redirect=${encodeURIComponent(
            redirectTo,
          )}`}
        >
          <Button
            size="large"
            startIcon={
              authMethods.oidc.iconUrl ? (
                <img
                  alt="Open ID Connect icon"
                  src={authMethods.oidc.iconUrl}
                  css={iconStyles}
                />
              ) : (
                <KeyIcon css={iconStyles} />
              )
            }
            disabled={isSigningIn}
            fullWidth
            type="submit"
          >
            {authMethods.oidc.signInText || Language.oidcSignIn}
          </Button>
        </Link>
      )}
    </Box>
  );
};
