import type {
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
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
import {
	AgentDevcontainerSSHButton,
	AgentSSHButton,
} from "./SSHButton/SSHButton";
import { TerminalLink } from "./TerminalLink/TerminalLink";
import { VSCodeDevContainerButton } from "./VSCodeDevContainerButton/VSCodeDevContainerButton";

type AgentDevcontainerCardProps = {
	agent: WorkspaceAgent;
	devcontainer: WorkspaceAgentDevcontainer;
	workspace: Workspace;
	wildcardHostname: string;
};

export const AgentDevcontainerCard: FC<AgentDevcontainerCardProps> = ({
	agent,
	devcontainer,
	workspace,
	wildcardHostname,
}) => {
	const [isRecreating, setIsRecreating] = useState(false);

	const handleRecreateDevcontainer = async () => {
		setIsRecreating(true);
		let recreateSucceeded = false;
		try {
			const response = await fetch(
				`/api/v2/workspaceagents/${agent.id}/containers/devcontainers/container/${devcontainer.container?.id}/recreate`,
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

	// If the devcontainer is starting, reflect this in the recreate button.
	useEffect(() => {
		if (devcontainer.status === "starting") {
			setIsRecreating(true);
		} else {
			setIsRecreating(false);
		}
	}, [devcontainer.status]);

	return (
		<section
			className="border border-border border-dashed rounded p-6 "
			key={devcontainer.id}
		>
			<header className="flex justify-between items-center mb-4">
				<div className="flex items-center gap-2">
					<h3 className="m-0 text-xs font-medium text-content-secondary">
						dev container:{" "}
						<span className="font-semibold">
							{devcontainer.name}
							{devcontainer.container && (
								<span className="text-content-tertiary">
									{" "}
									({devcontainer.container.name})
								</span>
							)}
						</span>
					</h3>
					{devcontainer.dirty && (
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

					{/* <AgentDevcontainerSSHButton
						workspace={workspace.name}
						container={devcontainer.container?.name || devcontainer.name}
					/> */}
					{/* TODO(mafredri): Sub agent display apps. */}
					{devcontainer.agent && agent.display_apps.includes("ssh_helper") && (
						<AgentSSHButton
							workspaceName={workspace.name}
							agentName={devcontainer.agent.name || devcontainer.name}
							workspaceOwnerUsername={workspace.owner_name}
						/>
					)}
				</div>
			</header>

			{devcontainer.agent && (
				<>
					<h4 className="m-0 text-xl font-semibold mb-2">Forwarded ports</h4>
					<div className="flex gap-4 flex-wrap mt-4">
						{devcontainer.container && (
							<VSCodeDevContainerButton
								userName={workspace.owner_name}
								workspaceName={workspace.name}
								devContainerName={devcontainer.container.name}
								devContainerFolder={devcontainer.agent.directory}
								displayApps={agent.display_apps} // TODO(mafredri): Sub agent display apps.
								agentName={agent.name} // This must be set to the parent agent.
							/>
						)}

						{devcontainer.agent && (
							<TerminalLink
								workspaceName={workspace.name}
								agentName={devcontainer.agent.name}
								userName={workspace.owner_name}
							/>
						)}

						{wildcardHostname !== "" &&
							devcontainer.container?.ports.map((port) => {
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
											devcontainer.agent?.name || agent.name,
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
				</>
			)}
		</section>
	);
};
