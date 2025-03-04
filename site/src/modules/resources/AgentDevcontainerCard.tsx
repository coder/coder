import Link from "@mui/material/Link";
import type { Workspace, WorkspaceAgentDevcontainer } from "api/typesGenerated";
import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { portForwardURL } from "utils/portForward";
import { AgentButton } from "./AgentButton";
import { AgentDevcontainerSSHButton } from "./SSHButton/SSHButton";
import { TerminalLink } from "./TerminalLink/TerminalLink";

type AgentDevcontainerCardProps = {
	container: WorkspaceAgentDevcontainer;
	workspace: Workspace;
	wildcardHostname: string;
	agentName: string;
};

export const AgentDevcontainerCard: FC<AgentDevcontainerCardProps> = ({
	container,
	workspace,
	agentName,
	wildcardHostname,
}) => {
	return (
		<section
			className="border border-border border-dashed rounded p-6 "
			key={container.id}
		>
			<header className="flex justify-between">
				<h3 className="m-0 text-xs font-medium text-content-secondary">
					{container.name}
				</h3>

				<AgentDevcontainerSSHButton
					workspace={workspace.name}
					container={container.name}
				/>
			</header>

			<h4 className="m-0 text-xl font-semibold">Forwarded ports</h4>

			<div className="flex gap-4 flex-wrap mt-4">
				<TerminalLink
					workspaceName={workspace.name}
					agentName={agentName}
					containerName={container.name}
					userName={workspace.owner_name}
				/>
				{wildcardHostname !== "" &&
					container.ports.map((port) => {
						return (
							<Link
								key={port.port}
								color="inherit"
								component={AgentButton}
								underline="none"
								startIcon={<ExternalLinkIcon className="size-icon-sm" />}
								href={portForwardURL(
									wildcardHostname,
									port.port,
									agentName,
									workspace.name,
									workspace.owner_name,
									location.protocol === "https" ? "https" : "http",
								)}
							>
								{port.process_name ||
									`${port.port}/${port.network.toUpperCase()}`}
							</Link>
						);
					})}
			</div>
		</section>
	);
};
