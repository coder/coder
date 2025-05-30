import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentContainer,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import { Spinner } from "components/Spinner/Spinner";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { ExternalLinkIcon } from "lucide-react";
import type { FC } from "react";
import { useEffect, useState } from "react";
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
	const [isRecreating, setIsRecreating] = useState(false);

	const handleRecreateDevcontainer = async () => {
		setIsRecreating(true);
		let recreateSucceeded = false;
		try {
			const response = await fetch(
				`/api/v2/workspaceagents/${agent.id}/containers/devcontainers/container/${container.id}/recreate`,
				{
					method: "POST",
				},
			);
			if (!response.ok) {
				const errorData = await response.json().catch(() => ({}));
				throw new Error(
					errorData.message || `Failed to recreate: ${response.statusText}`,
				);
			}
			// If the request was accepted (e.g. 202), we mark it as succeeded.
			// Once complete, the component will unmount, so the spinner will
			// disappear with it.
			if (response.status === 202) {
				recreateSucceeded = true;
			}
		} catch (error) {
			const errorMessage =
				error instanceof Error ? error.message : "An unknown error occurred.";
			displayError(`Failed to recreate devcontainer: ${errorMessage}`);
			console.error("Failed to recreate devcontainer:", error);
		} finally {
			if (!recreateSucceeded) {
				setIsRecreating(false);
			}
		}
	};

	// If the container is starting, reflect this in the recreate button.
	useEffect(() => {
		if (container.devcontainer_status === "starting") {
			setIsRecreating(true);
		} else {
			setIsRecreating(false);
		}
	}, [container.devcontainer_status]);

	return (
		<section
			className="border border-border border-dashed rounded p-6 "
			key={container.id}
		>
			<header className="flex justify-between items-center mb-4">
				<div className="flex items-center gap-2">
					<h3 className="m-0 text-xs font-medium text-content-secondary">
						dev container:{" "}
						<span className="font-semibold">{container.name}</span>
					</h3>
					{container.devcontainer_dirty && (
						<HelpTooltip>
							<HelpTooltipTrigger className="flex items-center text-xs text-content-warning ml-2">
								<span>Outdated</span>
							</HelpTooltipTrigger>
							<HelpTooltipContent>
								<HelpTooltipTitle>Devcontainer Outdated</HelpTooltipTitle>
								<HelpTooltipText>
									Devcontainer configuration has been modified and is outdated.
									Recreate to get an up-to-date container.
								</HelpTooltipText>
							</HelpTooltipContent>
						</HelpTooltip>
					)}
				</div>

				<div className="flex items-center gap-2">
					<Button
						variant="outline"
						size="sm"
						onClick={handleRecreateDevcontainer}
						disabled={isRecreating}
					>
						<Spinner loading={isRecreating} />
						Recreate
					</Button>

					<AgentDevcontainerSSHButton
						workspace={workspace.name}
						container={container.name}
					/>
				</div>
			</header>

			<h4 className="m-0 text-xl font-semibold mb-2">Forwarded ports</h4>

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
									<TooltipTrigger asChild>
										<AgentButton disabled={!hasHostBind} asChild>
											<a href={linkDest}>
												<ExternalLinkIcon />
												{portLabel}
											</a>
										</AgentButton>
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
