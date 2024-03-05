import Link from "@mui/material/Link";
import type { FC } from "react";
import type * as TypesGen from "api/typesGenerated";
import { TerminalIcon } from "components/Icons/TerminalIcon";
import { generateRandomString } from "utils/random";
import { AgentButton } from "../AgentButton";
import { DisplayAppNameMap } from "../AppLink/AppLink";

export const Language = {
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
export const TerminalLink: FC<TerminalLinkProps> = ({
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
      underline="none"
      color="inherit"
      component={AgentButton}
      startIcon={<TerminalIcon />}
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
      {DisplayAppNameMap["web_terminal"]}
    </Link>
  );
};
