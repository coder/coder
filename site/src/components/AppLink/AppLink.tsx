import Button from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import ComputerIcon from "@material-ui/icons/Computer"
import ErrorOutlineIcon from "@material-ui/icons/ErrorOutline"
import { FC, PropsWithChildren } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { generateRandomString } from "../../util/random"

export const Language = {
  appTitle: (appName: string, identifier: string): string =>
    `${appName} - ${identifier}`,
}

export interface AppLinkProps {
  appsHost?: string
  username: TypesGen.User["username"]
  workspaceName: TypesGen.Workspace["name"]
  agentName: TypesGen.WorkspaceAgent["name"]
  appName: TypesGen.WorkspaceApp["name"]
  appIcon?: TypesGen.WorkspaceApp["icon"]
  appCommand?: TypesGen.WorkspaceApp["command"]
  appSubdomain: TypesGen.WorkspaceApp["subdomain"]
  health: TypesGen.WorkspaceApp["health"]
}

export const AppLink: FC<PropsWithChildren<AppLinkProps>> = ({
  appsHost,
  username,
  workspaceName,
  agentName,
  appName,
  appIcon,
  appCommand,
  appSubdomain,
  health,
}) => {
  const styles = useStyles()

  // The backend redirects if the trailing slash isn't included, so we add it
  // here to avoid extra roundtrips.
  let href = `/@${username}/${workspaceName}.${agentName}/apps/${encodeURIComponent(
    appName,
  )}/`
  if (appCommand) {
    href = `/@${username}/${workspaceName}.${agentName}/terminal?command=${encodeURIComponent(
      appCommand,
    )}`
  }
  if (appsHost && appSubdomain) {
    const subdomain = `${appName}--${agentName}--${workspaceName}--${username}`
    href = `${window.location.protocol}//${subdomain}.${appsHost}/`
  }

  let canClick = true
  let icon = appIcon ? (
    <img alt={`${appName} Icon`} src={appIcon} />
  ) : (
    <ComputerIcon />
  )
  let tooltip = ""
  if (health === "initializing") {
    canClick = false
    icon = <CircularProgress size={16} />
    tooltip = "Initializing..."
  }
  if (health === "unhealthy") {
    canClick = false
    icon = <ErrorOutlineIcon className={styles.unhealthyIcon} />
    tooltip = "Unhealthy"
  }
  if (!appsHost && appSubdomain) {
    canClick = false
    icon = <ErrorOutlineIcon className={styles.notConfiguredIcon} />
    tooltip = "Your admin has not configured subdomain application access"
  }

  const button = (
    <Button
      size="small"
      startIcon={icon}
      className={styles.button}
      disabled={!canClick}
    >
      {appName}
    </Button>
  )

  return (
    <Tooltip title={tooltip}>
      <span>
        <Link
          href={href}
          target="_blank"
          className={canClick ? styles.link : styles.disabledLink}
          onClick={
            canClick
              ? (event) => {
                  event.preventDefault()
                  window.open(
                    href,
                    Language.appTitle(appName, generateRandomString(12)),
                    "width=900,height=600",
                  )
                }
              : undefined
          }
        >
          {button}
        </Link>
      </span>
    </Tooltip>
  )
}

const useStyles = makeStyles((theme) => ({
  link: {
    textDecoration: "none !important",
  },

  disabledLink: {
    pointerEvents: "none",
    textDecoration: "none !important",
  },

  button: {
    whiteSpace: "nowrap",
  },

  unhealthyIcon: {
    color: theme.palette.warning.light,
  },

  notConfiguredIcon: {
    color: theme.palette.grey[300],
  },
}))
