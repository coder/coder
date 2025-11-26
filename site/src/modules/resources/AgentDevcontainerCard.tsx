import Skeleton from "@mui/material/Skeleton";
import { API } from "api/api";
import type {
	Template,
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
	WorkspaceAgentListContainersResponse,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useProxy } from "contexts/ProxyContext";
import { Container, ExternalLinkIcon } from "lucide-react";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { AppStatuses } from "pages/WorkspacePage/AppStatuses";
import type { FC } from "react";
import { useEffect } from "react";
import { useMutation, useQueryClient } from "react-query";
import { cn } from "utils/cn";
import { portForwardURL } from "utils/portForward";
import { AgentApps, organizeAgentApps } from "./AgentApps/AgentApps";
import { AgentButton } from "./AgentButton";
import { AgentDevcontainerMoreActions } from "./AgentDevcontainerMoreActions";
import { AgentLatency } from "./AgentLatency";
import { DevcontainerStatus } from "./AgentStatus";
import { PortForwardButton } from "./PortForwardButton";
import { AgentSSHButton } from "./SSHButton/SSHButton";
import { SubAgentOutdatedTooltip } from "./SubAgentOutdatedTooltip";
import { TerminalLink } from "./TerminalLink/TerminalLink";
import { VSCodeDevContainerButton } from "./VSCodeDevContainerButton/VSCodeDevContainerButton";

type AgentDevcontainerCardProps = {
	parentAgent: WorkspaceAgent;
	subAgents: WorkspaceAgent[];
	devcontainer: WorkspaceAgentDevcontainer;
	workspace: Workspace;
	template: Template;
	wildcardHostname: string;
};

export const AgentDevcontainerCard: FC<AgentDevcontainerCardProps> = ({
	parentAgent,
	subAgents,
	devcontainer,
	workspace,
	template,
	wildcardHostname,
}) => {
	const { browser_only } = useFeatureVisibility();
	const { proxy } = useProxy();
	const queryClient = useQueryClient();

	// The sub agent comes from the workspace response whereas the devcontainer
	// comes from the agent containers endpoint. We need alignment between the
	// two, so if the sub agent is not present or the IDs do not match, we
	// assume it has been removed.
	const subAgent = subAgents.find((sub) => sub.id === devcontainer.agent?.id);

	const appSections = (subAgent && organizeAgentApps(subAgent.apps)) || [];
	const displayApps =
		subAgent?.display_apps.filter((app) => {
			if (browser_only) {
				return ["web_terminal", "port_forwarding_helper"].includes(app);
			}
			return true;
		}) || [];
	const showVSCode =
		devcontainer.container &&
		(displayApps.includes("vscode") || displayApps.includes("vscode_insiders"));
	const hasAppsToDisplay =
		displayApps.includes("web_terminal") ||
		showVSCode ||
		appSections.some((it) => it.apps.length > 0);

	const rebuildDevcontainerMutation = useMutation({
		mutationFn: async () => {
			await API.recreateDevContainer({
				parentAgentId: parentAgent.id,
				devcontainerId: devcontainer.id,
			});
		},
		onMutate: async () => {
			await queryClient.cancelQueries({
				queryKey: ["agents", parentAgent.id, "containers"],
			});

			// Snapshot the previous data for rollback in case of error.
			const previousData = queryClient.getQueryData([
				"agents",
				parentAgent.id,
				"containers",
			]);

			// Optimistically update the devcontainer status to
			// "starting" and zero the agent and container to mimic what
			// the API does.
			queryClient.setQueryData(
				["agents", parentAgent.id, "containers"],
				(oldData?: WorkspaceAgentListContainersResponse) => {
					if (!oldData?.devcontainers) return oldData;
					return {
						...oldData,
						devcontainers: oldData.devcontainers.map((dc) => {
							if (dc.id === devcontainer.id) {
								return {
									...dc,
									container: null,
									status: "starting",
								};
							}
							return dc;
						}),
					};
				},
			);

			return { previousData };
		},
		onError: (error, _, context) => {
			// If the mutation fails, use the context returned from
			// onMutate to roll back.
			if (context?.previousData) {
				queryClient.setQueryData(
					["agents", parentAgent.id, "containers"],
					context.previousData,
				);
			}
			const errorMessage =
				error instanceof Error ? error.message : "An unknown error occurred.";
			displayError(`Failed to rebuild devcontainer: ${errorMessage}`);
			console.error("Failed to rebuild devcontainer:", error);
		},
	});

	// Re-fetch containers when the subAgent changes to ensure data is
	// in sync. This relies on agent updates being pushed to the client
	// to trigger the re-fetch. That is why we match on name here
	// instead of ID as we need to fetch to get an up-to-date ID.
	const latestSubAgentByName = subAgents.find(
		(agent) => agent.name === devcontainer.name,
	);
	useEffect(() => {
		if (!latestSubAgentByName?.id || !latestSubAgentByName?.status) {
			return;
		}
		queryClient.invalidateQueries({
			queryKey: ["agents", parentAgent.id, "containers"],
		});
	}, [
		latestSubAgentByName?.id,
		latestSubAgentByName?.status,
		queryClient,
		parentAgent.id,
	]);

	const showDevcontainerControls = subAgent && devcontainer.container;
	const statusLabels: Partial<
		Record<WorkspaceAgentDevcontainer["status"], string>
	> = {
		deleting: "Deleting",
		stopping: "Stopping",
	};
	const rebuildButtonLabel =
		statusLabels[devcontainer.status] ??
		(devcontainer.container === undefined ? "Start" : "Rebuild");
	const isTransitioning =
		devcontainer.status === "starting" ||
		devcontainer.status === "stopping" ||
		devcontainer.status === "deleting";
	const showSubAgentApps =
		devcontainer.status !== "deleting" &&
		devcontainer.status !== "starting" &&
		subAgent?.status === "connected" &&
		hasAppsToDisplay;
	const showSubAgentAppsPlaceholders =
		devcontainer.status === "starting" || subAgent?.status === "connecting";

	const handleRebuildDevcontainer = () => {
		rebuildDevcontainerMutation.mutate();
	};

	const appsClasses = "flex flex-wrap gap-4 empty:hidden md:justify-start";

	return (
		<Stack
			key={devcontainer.id}
			direction="column"
			spacing={0}
			className={cn(
				"relative py-4 border border-dashed border-border rounded",
				devcontainer.error && "border-content-destructive border-solid",
			)}
		>
			<div
				className="absolute -top-2 left-5
				flex items-center gap-2
				bg-surface-primary px-2
				text-xs text-content-secondary"
			>
				<Container size={12} className="mr-1.5" />
				<span>dev container</span>
			</div>
			<header
				className="flex items-center justify-between flex-wrap
				gap-6 px-4 pl-8 leading-6
				md:gap-4"
			>
				<div className="flex items-center gap-6 text-xs text-content-secondary">
					<div className="flex items-center gap-4 md:w-full">
						<DevcontainerStatus
							devcontainer={devcontainer}
							parentAgent={parentAgent}
							agent={subAgent}
						/>
						<span
							className="max-w-xs shrink-0
							overflow-hidden text-ellipsis whitespace-nowrap
							text-sm font-semibold text-content-primary
							md:overflow-visible"
						>
							{subAgent?.name ??
								(devcontainer.name || devcontainer.config_path)}
							{devcontainer.container && (
								<span className="text-content-tertiary">
									{" "}
									({devcontainer.container.name})
								</span>
							)}
						</span>
					</div>
					{subAgent?.status === "connected" && (
						<>
							<SubAgentOutdatedTooltip
								devcontainer={devcontainer}
								agent={subAgent}
								onUpdate={handleRebuildDevcontainer}
							/>
							<AgentLatency agent={subAgent} />
						</>
					)}
					{subAgent?.status === "connecting" && (
						<>
							<Skeleton width={160} variant="text" />
							<Skeleton width={36} variant="text" />
						</>
					)}
				</div>

				<div className="flex items-center gap-2">
					<Button
						variant="outline"
						size="sm"
						onClick={handleRebuildDevcontainer}
						disabled={isTransitioning}
					>
						<Spinner loading={isTransitioning} />

						{rebuildButtonLabel}
					</Button>

					{showDevcontainerControls && displayApps.includes("ssh_helper") && (
						<AgentSSHButton
							workspaceName={workspace.name}
							agentName={subAgent.name}
							workspaceOwnerUsername={workspace.owner_name}
						/>
					)}
					{showDevcontainerControls &&
						displayApps.includes("port_forwarding_helper") &&
						proxy.preferredWildcardHostname !== "" && (
							<PortForwardButton
								host={proxy.preferredWildcardHostname}
								workspace={workspace}
								agent={subAgent}
								template={template}
							/>
						)}

					{showDevcontainerControls && (
						<AgentDevcontainerMoreActions
							devcontainer={devcontainer}
							parentAgent={parentAgent}
						/>
					)}
				</div>
			</header>

			{devcontainer.error && (
				<div className="px-8 pt-2 text-xs text-content-destructive">
					{devcontainer.error}
				</div>
			)}

			{(showSubAgentApps || showSubAgentAppsPlaceholders) && (
				<div className="flex flex-col gap-8 px-8 pt-4">
					{subAgent &&
						workspace.latest_app_status?.agent_id === subAgent.id && (
							<section>
								<h3 className="sr-only">App statuses</h3>
								<AppStatuses workspace={workspace} agent={subAgent} />
							</section>
						)}

					{showSubAgentApps && (
						<section className={appsClasses}>
							{showVSCode && (
								<VSCodeDevContainerButton
									userName={workspace.owner_name}
									workspaceName={workspace.name}
									devContainerName={devcontainer.container.name}
									devContainerFolder={subAgent?.directory ?? ""}
									localWorkspaceFolder={devcontainer.workspace_folder}
									localConfigFile={devcontainer.config_path || ""}
									displayApps={displayApps} // TODO(mafredri): We could use subAgent display apps here but we currently set none.
									agentName={parentAgent.name}
								/>
							)}
							{appSections.map((section, i) => (
								<AgentApps
									key={section.group ?? i}
									section={section}
									agent={subAgent}
									workspace={workspace}
								/>
							))}

							{displayApps.includes("web_terminal") && (
								<TerminalLink
									workspaceName={workspace.name}
									agentName={subAgent.name}
									userName={workspace.owner_name}
								/>
							)}

							{wildcardHostname !== "" &&
								devcontainer.container?.ports.map((port) => {
									const portLabel = `${
										port.port
									}/${port.network.toUpperCase()}`;
									const hasHostBind =
										port.host_port !== undefined && port.host_ip !== undefined;
									const helperText = hasHostBind
										? `${port.host_ip}:${port.host_port}`
										: "Not bound to host";
									const linkDest = hasHostBind
										? portForwardURL(
												wildcardHostname,
												port.host_port,
												subAgent.name,
												workspace.name,
												workspace.owner_name,
												location.protocol === "https" ? "https" : "http",
											)
										: "";
									return (
										<Tooltip key={portLabel}>
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
									);
								})}
						</section>
					)}

					{showSubAgentAppsPlaceholders && (
						<section className={appsClasses}>
							<Skeleton
								width={80}
								height={32}
								variant="rectangular"
								className="rounded"
							/>
							<Skeleton
								width={110}
								height={32}
								variant="rectangular"
								className="rounded"
							/>
						</section>
					)}
				</div>
			)}
		</Stack>
	);
};
