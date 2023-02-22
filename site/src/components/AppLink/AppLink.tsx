import Button from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import Tooltip from "@material-ui/core/Tooltip"
import ErrorOutlineIcon from "@material-ui/icons/ErrorOutline"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { generateRandomString } from "../../util/random"
import { BaseIcon } from "./BaseIcon"
import { ShareIcon } from "./ShareIcon"

const Language = {
  appTitle: (appName: string, identifier: string): string =>
    `${appName} - ${identifier}`,
}

export interface AppLinkProps {
  appsHost?: string
  workspace: TypesGen.Workspace
  app: TypesGen.WorkspaceApp
  agent: TypesGen.WorkspaceAgent
}

export const AppLink: FC<AppLinkProps> = ({
  appsHost,
  app,
  workspace,
  agent,
}) => {
  const styles = useStyles()
  const username = workspace.owner_name

  let appSlug = app.slug
  let appDisplayName = app.display_name
  if (!appSlug) {
    appSlug = appDisplayName
  }
  if (!appDisplayName) {
    appDisplayName = appSlug
  }

  // The backend redirects if the trailing slash isn't included, so we add it
  // here to avoid extra roundtrips.
  let href = `/@${username}/${workspace.name}.${
    agent.name
  }/apps/${encodeURIComponent(appSlug)}/`
  if (app.command) {
    href = `/@${username}/${workspace.name}.${
      agent.name
    }/terminal?command=${encodeURIComponent(app.command)}`
  }
  if (appsHost && app.subdomain) {
    const subdomain = `${appSlug}--${agent.name}--${workspace.name}--${username}`
    href = `${window.location.protocol}//${appsHost}/`.replace("*", subdomain)
  }
  if (app.external) {
    href = app.url
  }

  let canClick = true
  let icon = <BaseIcon app={app} />

  let primaryTooltip = ""
  if (app.health === "initializing") {
    canClick = false
    icon = <CircularProgress size={12} />
    primaryTooltip = "Initializing..."
  }
  if (app.health === "unhealthy") {
    canClick = false
    icon = <ErrorOutlineIcon className={styles.unhealthyIcon} />
    primaryTooltip = "Unhealthy"
  }
  if (!appsHost && app.subdomain) {
    canClick = false
    icon = <ErrorOutlineIcon className={styles.notConfiguredIcon} />
    primaryTooltip =
      "Your admin has not configured subdomain application access"
  }

  const button = (
    <Button
      size="small"
      startIcon={icon}
      endIcon={<ShareIcon app={app} />}
      className={styles.button}
      disabled={!canClick}
    >
      <span className={styles.appName}>{appDisplayName}</span>
    </Button>
  )

  return (
    <Tooltip title={primaryTooltip}>
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
                    Language.appTitle(appDisplayName, generateRandomString(12)),
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
    backgroundColor: theme.palette.background.default,

    "&:hover": {
      backgroundColor: `${theme.palette.background.default} !important`,
    },
  },

  unhealthyIcon: {
    color: theme.palette.warning.light,
  },

  notConfiguredIcon: {
    color: theme.palette.grey[300],
  },

  appName: {
    marginRight: theme.spacing(1),
  },
}))
