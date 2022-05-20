import Link from "@material-ui/core/Link"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"

export const Language = {
  linkText: "Open in terminal",
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
export const TerminalLink: React.FC<TerminalLinkProps> = ({ agentName, userName = "me", workspaceName }) => {
  return (
    <Link href={`/${userName}/${workspaceName}${agentName ? `.${agentName}` : ""}/terminal`} target="_blank">
      {Language.linkText}
    </Link>
  )
}
