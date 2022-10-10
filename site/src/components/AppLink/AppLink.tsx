import Button from "@material-ui/core/Button"
import CircularProgress from "@material-ui/core/CircularProgress"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import ComputerIcon from "@material-ui/icons/Computer"
import ErrorOutlineIcon from "@material-ui/icons/ErrorOutline"
import { FC, PropsWithChildren } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { generateRandomString } from "../../util/random"

export const Language = {
  appTitle: (appName: string, identifier: string): string => `${appName} - ${identifier}`,
}

export interface AppLinkProps {
  userName: TypesGen.User["username"]
  workspaceName: TypesGen.Workspace["name"]
  agentName: TypesGen.WorkspaceAgent["name"]
  appName: TypesGen.WorkspaceApp["name"]
  appIcon?: TypesGen.WorkspaceApp["icon"]
  appCommand?: TypesGen.WorkspaceApp["command"]
  health: TypesGen.WorkspaceApp["health"]
}

export const AppLink: FC<PropsWithChildren<AppLinkProps>> = ({
  userName,
  workspaceName,
  agentName,
  appName,
  appIcon,
  appCommand,
  health,
}) => {
  const styles = useStyles()

  // The backend redirects if the trailing slash isn't included, so we add it
  // here to avoid extra roundtrips.
  let href = `/@${userName}/${workspaceName}.${agentName}/apps/${encodeURIComponent(appName)}/`
  if (appCommand) {
    href = `/@${userName}/${workspaceName}.${agentName}/terminal?command=${encodeURIComponent(
      appCommand,
    )}`
  }

  let canClick = true
  let icon = appIcon ? <img alt={`${appName} Icon`} src={appIcon} /> : <ComputerIcon />
  if (health === "initializing") {
    canClick = false
    icon = <CircularProgress size={16} />
  }
  if (health === "unhealthy") {
    canClick = false
    icon = <ErrorOutlineIcon className={styles.unhealthyIcon} />
  }

  return (
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
      <Button size="small" startIcon={icon} className={styles.button} disabled={!canClick}>
        {appName}
      </Button>
    </Link>
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
}))
