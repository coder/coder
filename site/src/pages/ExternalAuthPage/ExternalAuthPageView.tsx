import { type Interpolation, type Theme } from "@emotion/react";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";
import RefreshIcon from "@mui/icons-material/Refresh";
import CircularProgress from "@mui/material/CircularProgress";
import Link from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import type { ApiErrorResponse } from "api/errors";
import type { ExternalAuth, ExternalAuthDevice } from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { Avatar } from "components/Avatar/Avatar";
import { CopyButton } from "components/CopyButton/CopyButton";
import { SignInLayout } from "components/SignInLayout/SignInLayout";
import { Welcome } from "components/Welcome/Welcome";
import { type FC, type ReactNode } from "react";

export interface ExternalAuthPageViewProps {
  externalAuth: ExternalAuth;
  viewExternalAuthConfig: boolean;

  externalAuthDevice?: ExternalAuthDevice;
  deviceExchangeError?: ApiErrorResponse;

  onReauthenticate: () => void;
}

const ExternalAuthPageView: FC<ExternalAuthPageViewProps> = ({
  deviceExchangeError,
  externalAuth,
  externalAuthDevice,
  onReauthenticate,
  viewExternalAuthConfig,
}) => {
  if (!externalAuth.authenticated) {
    return (
      <SignInLayout>
        <Welcome message={`Authenticate with ${externalAuth.display_name}`} />

        {externalAuth.device && (
          <GitDeviceAuth
            deviceExchangeError={deviceExchangeError}
            externalAuthDevice={externalAuthDevice}
          />
        )}
      </SignInLayout>
    );
  }

  const hasInstallations = externalAuth.installations.length > 0;

  // We only want to wrap this with a link if an install URL is available!
  let installTheApp: ReactNode = `install the ${externalAuth.display_name} App`;
  if (externalAuth.app_install_url) {
    installTheApp = (
      <Link
        href={externalAuth.app_install_url}
        target="_blank"
        rel="noreferrer"
      >
        {installTheApp}
      </Link>
    );
  }

  return (
    <SignInLayout>
      <Welcome
        message={`You've authenticated with ${externalAuth.display_name}!`}
      />
      <p css={styles.text}>
        {externalAuth.user?.login && `Hey @${externalAuth.user?.login}! 👋 `}
        {(!externalAuth.app_installable ||
          externalAuth.installations.length > 0) &&
          "You are now authenticated. Feel free to close this window!"}
      </p>

      {externalAuth.installations.length > 0 && (
        <div css={styles.authorizedInstalls}>
          {externalAuth.installations.map((install) => {
            if (!install.account) {
              return;
            }
            return (
              <Tooltip key={install.id} title={install.account.login}>
                <Link
                  href={install.account.profile_url}
                  target="_blank"
                  rel="noreferrer"
                >
                  <Avatar
                    size="sm"
                    src={install.account.avatar_url}
                    colorScheme="darken"
                  >
                    {install.account.login}
                  </Avatar>
                </Link>
              </Tooltip>
            );
          })}
          &nbsp;
          {externalAuth.installations.length} organization
          {externalAuth.installations.length !== 1 && "s are"} authorized
        </div>
      )}

      <div css={styles.links}>
        {!hasInstallations && externalAuth.app_installable && (
          <Alert severity="warning" css={styles.installAlert}>
            You must {installTheApp} to clone private repositories. Accounts
            will appear here once authorized.
          </Alert>
        )}

        {viewExternalAuthConfig &&
          externalAuth.app_install_url &&
          externalAuth.app_installable && (
            <Link
              href={externalAuth.app_install_url}
              target="_blank"
              rel="noreferrer"
              css={styles.link}
            >
              <OpenInNewIcon fontSize="small" />
              {externalAuth.installations.length > 0
                ? "Configure"
                : "Install"}{" "}
              the {externalAuth.display_name} App
            </Link>
          )}
        <Link
          css={styles.link}
          href="#"
          onClick={() => {
            onReauthenticate();
          }}
        >
          <RefreshIcon /> Reauthenticate
        </Link>
      </div>
    </SignInLayout>
  );
};

const GitDeviceAuth: FC<{
  externalAuthDevice?: ExternalAuthDevice;
  deviceExchangeError?: ApiErrorResponse;
}> = ({ externalAuthDevice, deviceExchangeError }) => {
  let status = (
    <p css={styles.status}>
      <CircularProgress size={16} color="secondary" data-chromatic="ignore" />
      Checking for authentication...
    </p>
  );
  if (deviceExchangeError) {
    // See https://datatracker.ietf.org/doc/html/rfc8628#section-3.5
    switch (deviceExchangeError.detail) {
      case "authorization_pending":
        break;
      case "expired_token":
        status = (
          <Alert severity="error">
            The one-time code has expired. Refresh to get a new one!
          </Alert>
        );
        break;
      case "access_denied":
        status = (
          <Alert severity="error">Access to the Git provider was denied.</Alert>
        );
        break;
      default:
        status = (
          <Alert severity="error">
            An unknown error occurred. Please try again:{" "}
            {deviceExchangeError.message}
          </Alert>
        );
        break;
    }
  }

  if (!externalAuthDevice) {
    return <CircularProgress />;
  }

  return (
    <div>
      <p css={styles.text}>
        Copy your one-time code:&nbsp;
        <div css={styles.copyCode}>
          <span css={styles.code}>{externalAuthDevice.user_code}</span>
          &nbsp; <CopyButton text={externalAuthDevice.user_code} />
        </div>
        <br />
        Then open the link below and paste it:
      </p>
      <div css={styles.links}>
        <Link
          css={styles.link}
          href={externalAuthDevice.verification_uri}
          target="_blank"
          rel="noreferrer"
        >
          <OpenInNewIcon fontSize="small" />
          Open and Paste
        </Link>
      </div>

      {status}
    </div>
  );
};

export default ExternalAuthPageView;

const styles = {
  text: (theme) => ({
    fontSize: 16,
    color: theme.palette.text.secondary,
    textAlign: "center",
    lineHeight: "160%",
    margin: 0,
  }),

  copyCode: {
    display: "inline-flex",
    alignItems: "center",
  },

  code: (theme) => ({
    fontWeight: "bold",
    color: theme.palette.text.primary,
  }),

  installAlert: {
    margin: 16,
  },

  links: {
    display: "flex",
    gap: 4,
    margin: 16,
    flexDirection: "column",
  },

  link: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    fontSize: 16,
    gap: 8,
  },

  status: (theme) => ({
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    gap: 8,
    color: theme.palette.text.disabled,
  }),

  authorizedInstalls: (theme) => ({
    display: "flex",
    gap: 4,
    color: theme.palette.text.disabled,
    margin: 32,
  }),
} satisfies Record<string, Interpolation<Theme>>;
