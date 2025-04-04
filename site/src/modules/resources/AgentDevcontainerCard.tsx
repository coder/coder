import Link from "@mui/material/Link";
import Tooltip, { type TooltipProps } from "@mui/material/Tooltip";
import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentContainer,
} from "api/typesGenerated";
import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { portForwardURL } from "utils/portForward";
import { AgentButton } from "./AgentButton";
import { AgentDevcontainerSSHButton } from "./SSHButton/SSHButton";
import { TerminalLink } from "./TerminalLink/TerminalLink";
import { VSCodeDevContainerButton } from "./VSCodeDevContainerButton/VSCodeDevContainerButton";

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
									port.host_port!,
									agent.name,
									workspace.name,
									workspace.owner_name,
									location.protocol === "https" ? "https" : "http",
								)
							: "";
						return (
							<Tooltip key={portLabel} title={helperText}>
								<span>
									<Link
										key={portLabel}
										color="inherit"
										component={AgentButton}
										underline="none"
										startIcon={<ExternalLinkIcon className="size-icon-sm" />}
										disabled={!hasHostBind}
										href={linkDest}
									>
										{portLabel}
									</Link>
								</span>
							</Tooltip>
						);
					})}
			</div>
		</section>
	);
};
