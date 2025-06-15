import type { Interpolation, Theme } from "@emotion/react";
import Skeleton from "@mui/material/Skeleton";
import type {
	Template,
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import { Spinner } from "components/Spinner/Spinner";
import { Stack } from "components/Stack/Stack";
import {
	Tooltip,
	TooltipContent,
	TooltipProvider,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { useProxy } from "contexts/ProxyContext";
import { Container, ExternalLinkIcon } from "lucide-react";
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { AppStatuses } from "pages/WorkspacePage/AppStatuses";
import type { FC } from "react";
import { useEffect, useState } from "react";
import { portForwardURL } from "utils/portForward";
import { Apps, organizeAgentApps } from "./AgentApps/AgentApps";
import { AgentButton } from "./AgentButton";
import { AgentLatency } from "./AgentLatency";
import { SubAgentStatus } from "./AgentStatus";
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

	const [isRebuilding, setIsRebuilding] = useState(false);

	// Track sub agent removal state to improve UX. This will not be needed once
	// the devcontainer and agent responses  are aligned.
	const [subAgentRemoved, setSubAgentRemoved] = useState(false);

	// The sub agent comes from the workspace response whereas the devcontainer
	// comes from the agent containers endpoint. We need alignment between the
	// two, so if the sub agent is not present or the IDs do not match, we
	// assume it has been removed.
	const subAgent = subAgents.find((sub) => sub.id === devcontainer.agent?.id);

	const appSections = (subAgent && organizeAgentApps(subAgent.apps)) || [];
	const displayApps =
		subAgent?.display_apps.filter((app) => {
			switch (true) {
				case browser_only:
					return ["web_terminal", "port_forwarding_helper"].includes(app);
				default:
					return true;
			}
		}) || [];
	const showVSCode =
		devcontainer.container &&
		(displayApps.includes("vscode") || displayApps.includes("vscode_insiders"));
	const hasAppsToDisplay =
		displayApps.includes("web_terminal") ||
		showVSCode ||
		appSections.some((it) => it.apps.length > 0);

	const showDevcontainerControls =
		!subAgentRemoved && subAgent && devcontainer.container;
	const showSubAgentApps =
		!subAgentRemoved && subAgent?.status === "connected" && hasAppsToDisplay;
	const showSubAgentAppsPlaceholders =
		subAgentRemoved || subAgent?.status === "connecting";

	const handleRebuildDevcontainer = async () => {
		setIsRebuilding(true);
		setSubAgentRemoved(true);
		let rebuildSucceeded = false;
		try {
			const response = await fetch(
				`/api/v2/workspaceagents/${parentAgent.id}/containers/devcontainers/container/${devcontainer.container?.id}/recreate`,
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
				rebuildSucceeded = true;
			}
		} catch (error) {
			const errorMessage =
				error instanceof Error ? error.message : "An unknown error occurred.";
			displayError(`Failed to recreate devcontainer: ${errorMessage}`);
			console.error("Failed to recreate devcontainer:", error);
		} finally {
			if (!rebuildSucceeded) {
				setIsRebuilding(false);
			}
		}
	};

	useEffect(() => {
		if (subAgent?.id) {
			setSubAgentRemoved(false);
		} else {
			setSubAgentRemoved(true);
		}
	}, [subAgent?.id]);

	// If the devcontainer is starting, reflect this in the recreate button.
	useEffect(() => {
		if (devcontainer.status === "starting") {
			setIsRebuilding(true);
		} else {
			setIsRebuilding(false);
		}
	}, [devcontainer]);

	return (
		<Stack
			key={devcontainer.id}
			direction="column"
			spacing={0}
			css={styles.devcontainerRow}
			className="border border-border border-dashed rounded relative"
		>
			<div
				css={styles.devContainerLabel}
				className="flex items-center gap-2 text-content-secondary"
			>
				<Container css={styles.devContainerIcon} size={12} />
				<span>dev container</span>
			</div>
			<header css={styles.header}>
				<div css={styles.agentInfo}>
					<div css={styles.agentNameAndStatus}>
						<SubAgentStatus agent={subAgent} />
						<span css={styles.agentName}>
							{subAgent?.name ?? devcontainer.name}
							{!isRebuilding && devcontainer.container && (
								<span className="text-content-tertiary">
									{" "}
									({devcontainer.container.name})
								</span>
							)}
						</span>
					</div>
					{!subAgentRemoved && subAgent?.status === "connected" && (
						<>
							<SubAgentOutdatedTooltip
								devcontainer={devcontainer}
								agent={subAgent}
								onUpdate={handleRebuildDevcontainer}
							/>
							<AgentLatency agent={subAgent} />
						</>
					)}
					{!subAgentRemoved && subAgent?.status === "connecting" && (
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
						disabled={isRebuilding}
					>
						<Spinner loading={isRebuilding} />
						Rebuild
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
				</div>
			</header>

			<div css={styles.content}>
				{subAgent && workspace.latest_app_status?.agent_id === subAgent.id && (
					<section>
						<h3 className="sr-only">App statuses</h3>
						<AppStatuses workspace={workspace} agent={subAgent} />
					</section>
				)}

				{showSubAgentApps && (
					<section css={styles.apps}>
						<>
							{showVSCode && (
								<VSCodeDevContainerButton
									userName={workspace.owner_name}
									workspaceName={workspace.name}
									devContainerName={devcontainer.container.name}
									devContainerFolder={subAgent?.directory ?? ""}
									displayApps={displayApps} // TODO(mafredri): We could use subAgent display apps here but we currently set none.
									agentName={parentAgent.name}
								/>
							)}
							{appSections.map((section, i) => (
								<Apps
									key={section.group ?? i}
									section={section}
									agent={subAgent}
									workspace={workspace}
								/>
							))}
						</>

						{displayApps.includes("web_terminal") && (
							<TerminalLink
								workspaceName={workspace.name}
								agentName={subAgent.name}
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
											subAgent.name,
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
					</section>
				)}

				{showSubAgentAppsPlaceholders && (
					<section css={styles.apps}>
						<Skeleton
							width={80}
							height={32}
							variant="rectangular"
							css={styles.buttonSkeleton}
						/>
						<Skeleton
							width={110}
							height={32}
							variant="rectangular"
							css={styles.buttonSkeleton}
						/>
					</section>
				)}
			</div>
		</Stack>
	);
};

const styles = {
	devContainerLabel: (theme) => ({
		backgroundColor: theme.palette.background.default,
		fontSize: 12,
		lineHeight: 1,
		padding: "4px 8px",
		position: "absolute",
		top: -11,
		left: 20,
	}),
	devContainerIcon: {
		marginRight: 5,
	},

	devcontainerRow: {
		padding: "16px 0px",
	},

	// Many of these styles are borrowed or mimic those from AgentRow.tsx.
	header: (theme) => ({
		padding: "0px 16px 0px 32px",
		display: "flex",
		gap: 24,
		alignItems: "center",
		justifyContent: "space-between",
		flexWrap: "wrap",
		lineHeight: "1.5",

		"&:has(+ [role='alert'])": {
			paddingBottom: 16,
		},

		[theme.breakpoints.down("md")]: {
			gap: 16,
		},
	}),

	agentInfo: (theme) => ({
		display: "flex",
		alignItems: "center",
		gap: 24,
		color: theme.palette.text.secondary,
		fontSize: 12,
	}),

	content: {
		padding: "16px 32px 0px 32px",
		display: "flex",
		flexDirection: "column",
		gap: 32,
	},

	apps: (theme) => ({
		display: "flex",
		gap: 16,
		flexWrap: "wrap",

		"&:empty": {
			display: "none",
		},

		[theme.breakpoints.down("md")]: {
			marginLeft: 0,
			justifyContent: "flex-start",
		},
	}),

	agentNameAndStatus: (theme) => ({
		display: "flex",
		alignItems: "center",
		gap: 16,

		[theme.breakpoints.down("md")]: {
			width: "100%",
		},
	}),

	agentName: (theme) => ({
		whiteSpace: "nowrap",
		overflow: "hidden",
		textOverflow: "ellipsis",
		maxWidth: 260,
		fontWeight: 600,
		flexShrink: 0,
		width: "fit-content",
		fontSize: 14,
		color: theme.palette.text.primary,

		[theme.breakpoints.down("md")]: {
			overflow: "unset",
		},
	}),

	buttonSkeleton: {
		borderRadius: 4,
	},
} satisfies Record<string, Interpolation<Theme>>;
