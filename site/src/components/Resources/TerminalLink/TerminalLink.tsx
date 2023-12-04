import Link from "@mui/material/Link";
import { AgentButton } from "components/Resources/AgentButton";
import { FC } from "react";
import * as TypesGen from "api/typesGenerated";
import { generateRandomString } from "utils/random";

export const Language = {
  linkText: "Terminal",
  terminalTitle: (identifier: string): string => `Terminal - ${identifier}`,
};

export interface TerminalLinkProps {
  agentName?: TypesGen.WorkspaceAgent["name"];
  userName?: TypesGen.User["username"];
  workspaceName: TypesGen.Workspace["name"];
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
  // Always use the primary for the terminal link. This is a relative link.
  const href = `/@${userName}/${workspaceName}${
    agentName ? `.${agentName}` : ""
  }/terminal`;

  return (
    <Link
      href={href}
      target="_blank"
      onClick={(event) => {
        event.preventDefault();
        window.open(
          href,
          Language.terminalTitle(generateRandomString(12)),
          "width=900,height=600",
        );
      }}
      data-testid="terminal"
    >
      <AgentButton>{Language.linkText}</AgentButton>
    </Link>
  );
};
