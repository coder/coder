import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentContainer,
} from "api/typesGenerated";
import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { portForwardURL } from "utils/portForward";
import { AgentDevcontainerSSHButton } from "./SSHButton/SSHButton";
import { TerminalLink } from "./TerminalLink/TerminalLink";
import { VSCodeDevContainerButton } from "./VSCodeDevContainerButton/VSCodeDevContainerButton";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { Button } from "components/Button/Button";

type AgentDevcontainerCardProps = {
	agent: WorkspaceAgent;
	container: WorkspaceAgentContainer;
	workspace: Workspace;
	wildcardHostname: string;
};

export const AgentDevcontainerCard: FC<AgentDevcontainerCardProps> = ({
	agent,
	container,
	workspace,
	wildcardHostname,
}) => {
	const folderPath = container.labels["devcontainer.local_folder"];
	const containerFolder = container.volumes[folderPath];

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
				<VSCodeDevContainerButton
					userName={workspace.owner_name}
					workspaceName={workspace.name}
					devContainerName={container.name}
					devContainerFolder={containerFolder}
					displayApps={agent.display_apps}
					agentName={agent.name}
				/>

				<TerminalLink
					workspaceName={workspace.name}
					agentName={agent.name}
					containerName={container.name}
					userName={workspace.owner_name}
				/>
				{wildcardHostname !== "" &&
					container.ports.map((port) => {
						const portLabel = `${port.port}/${port.network.toUpperCase()}`;
						const hasHostBind =
							port.host_port !== undefined && port.host_ip !== undefined;
						const helperText = hasHostBind
							? `${port.host_ip}:${port.host_port}`
							: "Not bound to host";
						const linkDest = hasHostBind
							? portForwardURL(
									wildcardHostname,
									port.host_port,
									agent.name,
									workspace.name,
									workspace.owner_name,
									location.protocol === "https" ? "https" : "http",
								)
							: "";
						return (
							<TooltipProvider key={portLabel}>
								<Tooltip>
									<TooltipTrigger>
										<Button disabled={!hasHostBind} asChild>
											<a href={linkDest}>
												<ExternalLinkIcon />
												{portLabel}
											</a>
										</Button>
									</TooltipTrigger>
									<TooltipContent>{helperText}</TooltipContent>
								</Tooltip>
							</TooltipProvider>
						);
					})}
			</div>
		</section>
	);
};
