import Link from "@material-ui/core/Link"
import { makeStyles } from "@material-ui/core/styles"
import ComputerIcon from "@material-ui/icons/Computer"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { combineClasses } from "../../util/combineClasses"
import { generateRandomString } from "../../util/random"

export const Language = {
  linkText: "Terminal",
  terminalTitle: (identifier: string): string => `Terminal - ${identifier}`,
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
export const TerminalLink: FC<TerminalLinkProps> = ({ agentName, userName = "me", workspaceName, className }) => {
  const styles = useStyles()
  const href = `/@${userName}/${workspaceName}${agentName ? `.${agentName}` : ""}/terminal`

  return (
    <Link
      href={href}
      className={combineClasses([styles.link, className])}
      target="_blank"
      onClick={(event) => {
        event.preventDefault()
        window.open(href, Language.terminalTitle(generateRandomString(12)), "width=900,height=600")
      }}
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
