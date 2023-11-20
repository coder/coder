import Button from "@mui/material/Button";
import GitHubIcon from "@mui/icons-material/GitHub";
import KeyIcon from "@mui/icons-material/VpnKey";
import Box from "@mui/material/Box";
import { type FC } from "react";
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
  const iconStyles = {
    width: 16,
    height: 16,
  };

  return (
    <Box display="grid" gap="16px">
      {authMethods?.github.enabled && (
        <Button
          component="a"
          href={`/api/v2/users/oauth2/github/callback?redirect=${encodeURIComponent(
            redirectTo,
          )}`}
          variant="contained"
          startIcon={<GitHubIcon css={iconStyles} />}
          disabled={isSigningIn}
          fullWidth
          type="submit"
          size="xlarge"
        >
          {Language.githubSignIn}
        </Button>
      )}

      {authMethods?.oidc.enabled && (
        <Button
          component="a"
          href={`/api/v2/users/oidc/callback?redirect=${encodeURIComponent(
            redirectTo,
          )}`}
          variant="contained"
          size="xlarge"
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
      )}
    </Box>
  );
};
