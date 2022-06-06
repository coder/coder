import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import React, { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"

export interface AppLinkProps {
  userName: TypesGen.User["username"]
  workspaceName: TypesGen.Workspace["name"]
  appName: TypesGen.WorkspaceApp["name"]
  appIcon: TypesGen.WorkspaceApp["icon"]
}

export const AppLink: FC<AppLinkProps> = ({ userName, workspaceName, appName, appIcon }) => {
  const styles = useStyles()
  const href = `/@${userName}/${workspaceName}/apps/${appName}`

  return (
    <Link href={href} target="_blank" className={styles.link}>
      {appIcon && <img className={styles.icon} alt={`${appName} Icon`} src={appIcon} />}
      {appName}
    </Link>
  )
}

const useStyles = makeStyles((theme) => ({
  link: {
    color: theme.palette.text.secondary,
    display: "flex",
    alignItems: "center",
  },

  icon: {
    width: 16,
    height: 16,
    marginRight: theme.spacing(1.5),
  },
}))
