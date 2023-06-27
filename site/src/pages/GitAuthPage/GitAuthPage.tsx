import RefreshIcon from "@mui/icons-material/Refresh"
import OpenInNewIcon from "@mui/icons-material/OpenInNew"
import CircularProgress from "@mui/material/CircularProgress"
import Link from "@mui/material/Link"
import Tooltip from "@mui/material/Tooltip"
import { makeStyles } from "@mui/styles"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import {
  exchangeGitAuthDevice,
  getGitAuthDevice,
  getGitAuthProvider,
} from "api/api"
import { isAxiosError } from "axios"
import { Alert } from "components/Alert/Alert"
import { Avatar } from "components/Avatar/Avatar"
import { CopyButton } from "components/CopyButton/CopyButton"
import { SignInLayout } from "components/SignInLayout/SignInLayout"
import { Welcome } from "components/Welcome/Welcome"
import { FC, useEffect } from "react"
import { useParams } from "react-router-dom"
import { REFRESH_GITAUTH_BROADCAST_CHANNEL } from "xServices/createWorkspace/createWorkspaceXService"
import { usePermissions } from "hooks"

const GitAuthPage: FC = () => {
  const styles = useStyles()
  const { provider } = useParams()
  if (!provider) {
    throw new Error("provider must exist")
  }
  const permissions = usePermissions()
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: ["gitauth", provider],
    queryFn: () => getGitAuthProvider(provider),
    refetchOnWindowFocus: true,
  })

  useEffect(() => {
    if (!query.data?.authenticated) {
      return
    }
    // This is used to notify the parent window that the Git auth token has been refreshed.
    // It's critical in the create workspace flow!
    // eslint-disable-next-line compat/compat -- It actually is supported... not sure why it's complaining.
    const bc = new BroadcastChannel(REFRESH_GITAUTH_BROADCAST_CHANNEL)
    // The message doesn't matter, any message refreshes the page!
    bc.postMessage("noop")
  }, [query.data?.authenticated])

  if (query.isLoading || !query.data) {
    return null
  }

  if (!query.data.authenticated) {
    return (
      <SignInLayout>
        <Welcome message={`Authenticate with ${query.data.type}`} />

        {query.data.device && <GitDeviceAuth provider={provider} />}
      </SignInLayout>
    )
  }

  const hasInstallations = query.data.installations?.length > 0

  return (
    <SignInLayout>
      <Welcome message={`You've authenticated with ${query.data.type}!`} />
      <p className={styles.text}>
        Hey @{query.data.user?.login} 👋! You are now authenticated with Git.
        Feel free to close this window!
      </p>

      {query.data.installations?.length !== 0 && (
        <div className={styles.authorizedInstalls}>
          {query.data.installations.map((install) => {
            if (!install.account) {
              return
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
            )
          })}
          &nbsp;
          {query.data.installations.length} organization
          {query.data.installations.length !== 1 && "s are"} authorized
        </div>
      )}

      <div className={styles.links}>
        {!hasInstallations && query.data.app_installable && (
          <Alert severity="warning" className={styles.installAlert}>
            You must{" "}
            <Link
              href={query.data.app_install_url}
              target="_blank"
              rel="noreferrer"
            >
              install the {query.data.type} App
            </Link>{" "}
            to clone private repositories. Accounts will appear here once
            authorized.
          </Alert>
        )}

        {permissions.viewGitAuthConfig &&
          query.data.app_install_url &&
          query.data.app_installable && (
            <Link
              href={query.data.app_install_url}
              target="_blank"
              rel="noreferrer"
              className={styles.link}
            >
              <OpenInNewIcon fontSize="small" />
              {query.data.installations?.length
                ? "Configure"
                : "Install"} the {query.data.type} App
            </Link>
          )}
        <Link
          className={styles.link}
          href="#"
          onClick={() => {
            queryClient.setQueryData(["gitauth", provider], {
              ...query.data,
              authenticated: false,
            })
          }}
        >
          <RefreshIcon /> Reauthenticate
        </Link>
      </div>
    </SignInLayout>
  )
}

const GitDeviceAuth: FC<{
  provider: string
}> = ({ provider }) => {
  const styles = useStyles()
  const device = useQuery({
    queryFn: () => getGitAuthDevice(provider),
    queryKey: ["gitauth", provider, "device"],
    refetchOnMount: false,
  })

  const client = useQueryClient()
  const exchange = useQuery({
    queryFn: () =>
      exchangeGitAuthDevice(provider, {
        device_code: device.data?.device_code || "",
      }),
    queryKey: ["gitauth", provider, device.data?.device_code],
    enabled: Boolean(device.data),
    onSuccess: () => {
      // Force a refresh of the Git auth status.
      client.invalidateQueries(["gitauth", provider]).catch((ex) => {
        console.error("invalidate queries", ex)
      })
    },
    retry: true,
    retryDelay: (device.data?.interval || 5) * 1000,
    refetchOnWindowFocus: (query) =>
      query.state.status === "success" ? false : "always",
  })

  let status = (
    <p className={styles.status}>
      <CircularProgress size={16} color="secondary" />
      Checking for authentication...
    </p>
  )
  if (isAxiosError(exchange.failureReason)) {
    // See https://datatracker.ietf.org/doc/html/rfc8628#section-3.5
    switch (exchange.failureReason.response?.data?.detail) {
      case "authorization_pending":
        break
      case "expired_token":
        status = (
          <Alert severity="error">
            The one-time code has expired. Refresh to get a new one!
          </Alert>
        )
        break
      case "access_denied":
        status = (
          <Alert severity="error">Access to the Git provider was denied.</Alert>
        )
        break
      default:
        status = (
          <Alert severity="error">
            An unknown error occurred. Please try again:{" "}
            {exchange.failureReason.message}
          </Alert>
        )
        break
    }
  }

  if (!device.data) {
    return <CircularProgress />
  }

  return (
    <div>
      <p className={styles.text}>
        Copy your one-time code:&nbsp;
        <div className={styles.copyCode}>
          <span className={styles.code}>{device.data.user_code}</span>&nbsp;{" "}
          <CopyButton text={device.data.user_code} />
        </div>
        <br />
        Then open the link below and paste it:
      </p>
      <div className={styles.links}>
        <Link
          className={styles.link}
          href={device.data.verification_uri}
          target="_blank"
          rel="noreferrer"
        >
          <OpenInNewIcon fontSize="small" />
          Open and Paste
        </Link>
      </div>

      {status}
    </div>
  )
}

export default GitAuthPage

const useStyles = makeStyles((theme) => ({
  text: {
    fontSize: 16,
    color: theme.palette.text.secondary,
    textAlign: "center",
    lineHeight: "160%",
    margin: 0,
  },

  copyCode: {
    display: "inline-flex",
    alignItems: "center",
  },

  code: {
    fontWeight: "bold",
    color: theme.palette.text.primary,
  },

  installAlert: {
    margin: theme.spacing(2),
  },

  links: {
    display: "flex",
    gap: theme.spacing(0.5),
    margin: theme.spacing(2),
    flexDirection: "column",
  },

  link: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    fontSize: 16,
    gap: theme.spacing(1),
  },

  status: {
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    gap: theme.spacing(1),
    color: theme.palette.text.disabled,
  },

  authorizedInstalls: {
    display: "flex",
    gap: 4,
    color: theme.palette.text.disabled,
    margin: theme.spacing(4),
  },
}))
