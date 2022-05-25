import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import ComputerIcon from "@material-ui/icons/Computer"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { combineClasses } from "../../util/combineClasses"

export const Language = {
  linkText: "Open terminal",
}

export interface TerminalLinkProps {
  agentName?: TypesGen.WorkspaceAgent["name"]
  userName?: TypesGen.User["username"]
  workspaceName: TypesGen.Workspace["name"]
  className?: string
}

/**
 * Generate a link to a terminal connected to the provided workspace agent.  If
 * no agent is provided connect to the first agent.
 *
 * If no user name is provided "me" is used however it makes the link not
 * shareable.
 */
export const TerminalLink: React.FC<TerminalLinkProps> = ({ agentName, userName = "me", workspaceName, className }) => {
  const styles = useStyles()

  return (
    <Link
      href={`/${userName}/${workspaceName}${agentName ? `.${agentName}` : ""}/terminal`}
      className={combineClasses([styles.link, className])}
      target="_blank"
    >
      <ComputerIcon className={styles.icon} />
      {Language.linkText}
    </Link>
  )
}

// Replicating these from accessLink style from Resources component until we
// define if we want these styles coming from the parent or having a
// ResourceLink component for that
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
