import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import React, { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { combineClasses } from "../../util/combineClasses"

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
    <Link
      href={href}
      target="_blank"
      className={styles.link}
      onClick={(event) => {
        event.preventDefault()
        window.open(href, appName, "width=900,height=600")
      }}
    >
      <img
        className={combineClasses([styles.icon, appIcon === "" ? "empty" : ""])}
        alt={`${appName} Icon`}
        src={appIcon || ""}
      />
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

    // If no icon is provided we still want the padding on the left
    // to occur.
    "&.empty": {
      opacity: 0,
    },
  },
}))
