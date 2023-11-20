import Button from "@mui/material/Button";
import GitHubIcon from "@mui/icons-material/GitHub";
import KeyIcon from "@mui/icons-material/VpnKey";
import Box from "@mui/material/Box";
import { useId, type FC } from "react";
import { Language } from "./SignInForm";
import { type AuthMethods } from "api/typesGenerated";
import { visuallyHidden } from "@mui/utils";

type OAuthSignInFormProps = {
  isSigningIn: boolean;
  redirectTo: string;
  authMethods?: AuthMethods;
};

const iconStyles = {
  width: 16,
  height: 16,
};

export const OAuthSignInForm: FC<OAuthSignInFormProps> = ({
  isSigningIn,
  redirectTo,
  authMethods,
}) => {
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
              <OidcIcon iconUrl={authMethods.oidc.iconUrl} />
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

type OidcIconProps = {
  iconUrl: string;
};

function OidcIcon({ iconUrl }: OidcIconProps) {
  const hookId = useId();
  const oidcId = `${hookId}-oidc`;

  // Even if the URL is defined, there is a chance that the request for the
  // image fails. Have to use blank alt text to avoid button from getting ugly
  // if that happens, but also still need a way to inject accessible text
  return (
    <>
      <img alt="" src={iconUrl} css={iconStyles} aria-labelledby={oidcId} />
      <div id={oidcId} css={{ ...visuallyHidden }}>
        Open ID Connect
      </div>
    </>
  );
}
