import Link from "@material-ui/core/Link"
import { SecondaryAgentButton } from "components/Resources/AgentButton"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { generateRandomString } from "../../utils/random"
import { useProxy } from "contexts/ProxyContext"

export const Language = {
  linkText: "Terminal",
  terminalTitle: (identifier: string): string => `Terminal - ${identifier}`,
}

export interface TerminalLinkProps {
  agentName?: TypesGen.WorkspaceAgent["name"]
  userName?: TypesGen.User["username"]
  workspaceName: TypesGen.Workspace["name"]
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
}) => {
  const { proxy } = useProxy()

  const href = `${proxy.preferredPathAppURL}/@${userName}/${workspaceName}${agentName ? `.${agentName}` : ""
    }/terminal`

  return (
    <Link
      underline="none"
      href={href}
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
      <SecondaryAgentButton size="small" variant="outlined">
        {Language.linkText}
      </SecondaryAgentButton>
    </Link>
  )
}
