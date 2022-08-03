import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import ComputerIcon from "@material-ui/icons/Computer"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { generateRandomString } from "../../util/random"

export const Language = {
  appTitle: (appName: string, identifier: string): string => `${appName} - ${identifier}`,
}

export interface AppLinkProps {
  userName: TypesGen.User["username"]
  workspaceName: TypesGen.Workspace["name"]
  appName: TypesGen.WorkspaceApp["name"]
  appIcon?: TypesGen.WorkspaceApp["icon"]
}

export const AppLink: FC<PropsWithChildren<AppLinkProps>> = ({
  userName,
  workspaceName,
  appName,
  appIcon,
}) => {
  const styles = useStyles()
  const href = `/@${userName}/${workspaceName}/apps/${appName}`

  return (
    <Link
      href={href}
      target="_blank"
      className={styles.link}
      onClick={(event) => {
        event.preventDefault()
        window.open(
          href,
          Language.appTitle(appName, generateRandomString(12)),
          "width=900,height=600",
        )
      }}
    >
      <Button
        size="small"
        startIcon={appIcon ? <img alt={`${appName} Icon`} src={appIcon} /> : <ComputerIcon />}
        className={styles.button}
      >
        {appName}
      </Button>
    </Link>
  )
}

const useStyles = makeStyles(() => ({
  link: {
    textDecoration: "none !important",
  },

  button: {
    whiteSpace: "nowrap",
  },
}))
