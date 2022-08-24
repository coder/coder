import Button from "@material-ui/core/Button"
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
export const TerminalLink: FC<React.PropsWithChildren<TerminalLinkProps>> = ({
  agentName,
  userName = "me",
  workspaceName,
  className,
}) => {
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
      <Button startIcon={<ComputerIcon />} size="small">
        {Language.linkText}
      </Button>
    </Link>
  )
}

const useStyles = makeStyles(() => ({
  link: {
    textDecoration: "none !important",
  },
}))
