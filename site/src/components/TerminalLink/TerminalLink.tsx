import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { combineClasses } from "../../utils/combineClasses"
import { generateRandomString } from "../../utils/random"

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
  className = "",
}) => {
  const styles = useStyles()
  const href = `/@${userName}/${workspaceName}${
    agentName ? `.${agentName}` : ""
  }/terminal`

  return (
    <Button
      href={href}
      component="a"
      size="small"
      variant="outlined"
      className={combineClasses([styles.button, className])}
      target="_blank"
      onClick={(event) => {
        event.preventDefault()
        window.open(
          href,
          Language.terminalTitle(generateRandomString(12)),
          "width=900,height=600",
        )
      }}
    >
      {Language.linkText}
    </Button>
  )
}

const useStyles = makeStyles((theme) => ({
  button: {
    fontSize: 12,
    fontWeight: 500,
    height: theme.spacing(4),
    minHeight: theme.spacing(4),
    borderRadius: 4,
  },
}))
