import type { Interpolation, Theme } from "@emotion/react";
import { alpha } from "@mui/material/styles";
import type {
	Template,
	Workspace,
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
} from "api/typesGenerated";
import { Stack } from "components/Stack/Stack";
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
import { ExternalLinkIcon, Container } from "lucide-react";
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
import { useFeatureVisibility } from "modules/dashboard/useFeatureVisibility";
import { useProxy } from "contexts/ProxyContext";
import { PortForwardButton } from "./PortForwardButton";
import { AgentStatus, SubAgentStatus } from "./AgentStatus";
import { AgentVersion } from "./AgentVersion";
import { AgentLatency } from "./AgentLatency";
import Skeleton from "@mui/material/Skeleton";
import { AppStatuses } from "pages/WorkspacePage/AppStatuses";
import { VSCodeDesktopButton } from "./VSCodeDesktopButton/VSCodeDesktopButton";
import { SubAgentOutdatedTooltip } from "./SubAgentOutdatedTooltip";

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
	const [isRebuilding, setIsRebuilding] = useState(false);
	const [subAgentRemoved, setSubAgentRemoved] = useState(false);

	const { browser_only } = useFeatureVisibility();
	const { proxy } = useProxy();

	const subAgent = subAgents.find((sub) => sub.id === devcontainer.agent?.id);
	const shouldDisplaySubAgentApps =
		devcontainer.container && subAgent?.status === "connected";
	const shouldNotDisplaySubAgentApps = !devcontainer.container || !subAgent;

	const showSubAgentAppsAndElements =
		!subAgentRemoved &&
		subAgent &&
		subAgent.status === "connected" &&
		devcontainer.container;

	// const appSections = organizeAgentApps(subAgent?.apps);
	// const hasAppsToDisplay =
	// 	!browser_only || appSections.some((it) => it.apps.length > 0);
	const hasAppsToDisplay = true;
	const shouldDisplayAgentApps =
		(subAgent?.status === "connected" && hasAppsToDisplay) ||
		subAgent?.status === "connecting";
	const hasVSCodeApp =
		subAgent?.display_apps.includes("vscode") ||
		subAgent?.display_apps.includes("vscode_insiders");
	const showVSCode = hasVSCodeApp && !browser_only;

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
		// If the sub agent is removed, we set the state to true to avoid rendering it.
		if (!subAgent?.id) {
			setSubAgentRemoved(true);
		} else {
			setSubAgentRemoved(false);
		}
	}, [subAgent?.id]);

	// If the devcontainer is starting, reflect this in the recreate button.
	useEffect(() => {
		console.log(
			"Devcontainer status:",
			devcontainer.status,
			"Sub agent status:",
			subAgent?.status,
		);
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
			css={[styles.subAgentRow]}
			className="border border-border border-dashed rounded"
		>
			<div
				css={styles.devContainerLabel}
				className="flex items-center gap-2 text-content-secondary border border-border border-dashed rounded"
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

					{showSubAgentAppsAndElements &&
						subAgent.display_apps.includes("ssh_helper") && (
							<AgentSSHButton
								workspaceName={workspace.name}
								agentName={subAgent.name}
								workspaceOwnerUsername={workspace.owner_name}
							/>
						)}
					{showSubAgentAppsAndElements &&
						proxy.preferredWildcardHostname !== "" &&
						subAgent.display_apps.includes("port_forwarding_helper") && (
							<PortForwardButton
								host={proxy.preferredWildcardHostname}
								workspace={workspace}
								agent={subAgent}
								template={template}
							/>
						)}
				</div>
			</header>
			{/* <header className="flex justify-between items-center mb-4">
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
									Rebuild to get an up-to-date container.
								</HelpTooltipText>
							</HelpTooltipContent>
						</HelpTooltip>
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

					{shouldDisplaySubAgentApps &&
						!browser_only &&
						// TODO(mafredri): We could use subAgent display apps here but we currently set none.
						parentAgent.display_apps.includes("ssh_helper") && (
							<AgentSSHButton
								workspaceName={workspace.name}
								agentName={subAgent.name}
								workspaceOwnerUsername={workspace.owner_name}
							/>
						)}

					{shouldDisplaySubAgentApps &&
						proxy.preferredWildcardHostname === "" &&
						// TODO(mafredri): We could use subAgent display apps here but we currently set none.
						parentAgent.display_apps.includes("port_forwarding_helper") && (
							<PortForwardButton
								host={proxy.preferredWildcardHostname}
								workspace={workspace}
								agent={subAgent}
								template={template}
							/>
						)}
				</div>
			</header> */}

			<div css={styles.content}>
				{subAgent && workspace.latest_app_status?.agent_id === subAgent.id && (
					<section>
						<h3 className="sr-only">App statuses</h3>
						<AppStatuses workspace={workspace} agent={subAgent} />
					</section>
				)}

				<section css={styles.apps}>
					{showSubAgentAppsAndElements && (
						<>
							{showVSCode && (
								<VSCodeDevContainerButton
									userName={workspace.owner_name}
									workspaceName={workspace.name}
									devContainerName={devcontainer.container.name}
									devContainerFolder={subAgent.directory ?? "/"} // This will always be set on subagents but provide fallback anyway.
									displayApps={subAgent.display_apps} // TODO(mafredri): We could use subAgent display apps here but we currently set none.
									agentName={parentAgent.name}
								/>
							)}
							{/* {appSections.map((section, i) => (
								<Apps
									key={section.group ?? i}
									section={section}
									agent={agent}
									workspace={workspace}
								/>
							))} */}
						</>
					)}

					{showSubAgentAppsAndElements &&
						subAgent.display_apps.includes("web_terminal") && (
							<TerminalLink
								workspaceName={workspace.name}
								agentName={subAgent.name}
								userName={workspace.owner_name}
							/>
						)}
				</section>

				{/* <div className="flex gap-4 flex-wrap mt-4"> */}
				{/* {showApps && subAgent && devcontainer.container && (
					<VSCodeDevContainerButton
						userName={workspace.owner_name}
						workspaceName={workspace.name}
						devContainerName={devcontainer.container.name}
						devContainerFolder={subAgent.directory ?? "/"} // This will always be set on subagents but provide fallback anyway.
						displayApps={parentAgent.display_apps} // TODO(mafredri): We could use subAgent display apps here but we currently set none.
						agentName={parentAgent.name}
					/>
				)}

				{showApps && parentAgent.display_apps.includes("web_terminal") && (
					<TerminalLink
						workspaceName={workspace.name}
						agentName={subAgent.name}
						userName={workspace.owner_name}
					/>
				)} */}

				{showSubAgentAppsAndElements &&
					wildcardHostname !== "" &&
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

				{!showSubAgentAppsAndElements && (
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

			{/* </section> */}
		</Stack>
	);
};

const styles = {
	// Many of these styles are borrowed or mimic those from AgentRow.tsx.
	subAgentRow: (theme) => ({
		fontSize: 14 - 2,
		// TODO(mafredri): Not sure which color to use here, this comes
		// from the border css classes.
		border: `1px dashed hsl(var(--border-default))`,
		borderRadius: 8,
		position: "relative",
	}),

	header: (theme) => ({
		padding: "16px 16px 0 32px",
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

	devContainerLabel: (theme) => ({
		backgroundColor: theme.palette.background.default,
		fontSize: 12,
		lineHeight: 1,
		padding: "4px 8px",
		position: "absolute",
		top: -11,
		left: 19,
	}),
	devContainerIcon: {
		marginRight: 5,
	},

	agentInfo: (theme) => ({
		display: "flex",
		alignItems: "center",
		gap: 24,
		color: theme.palette.text.secondary,
		fontSize: 14 - 2,
	}),

	agentNameAndInfo: (theme) => ({
		display: "flex",
		alignItems: "center",
		gap: 24,
		flexWrap: "wrap",

		[theme.breakpoints.down("md")]: {
			gap: 12,
		},
	}),

	content: {
		padding: 32,
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

	agentDescription: (theme) => ({
		fontSize: 14 - 2,
		color: theme.palette.text.secondary,
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
		fontSize: 16 - 2,
		color: theme.palette.text.primary,

		[theme.breakpoints.down("md")]: {
			overflow: "unset",
		},
	}),

	agentDataGroup: {
		display: "flex",
		alignItems: "baseline",
		gap: 48,
	},

	agentData: (theme) => ({
		display: "flex",
		flexDirection: "column",
		fontSize: 12 - 2,

		"& > *:first-of-type": {
			fontWeight: 500,
			color: theme.palette.text.secondary,
		},
	}),

	buttonSkeleton: {
		borderRadius: 4,
	},

	agentErrorMessage: (theme) => ({
		fontSize: 12 - 2,
		fontWeight: 400,
		marginTop: 4,
		color: theme.palette.warning.light,
	}),

	agentOS: {
		textTransform: "capitalize",
	},

	startupLogs: (theme) => ({
		maxHeight: 256,
		borderBottom: `1px solid ${theme.palette.divider}`,
		backgroundColor: theme.palette.background.paper,
		paddingTop: 16,

		// We need this to be able to apply the padding top from startupLogs
		"& > div": {
			position: "relative",
		},
	}),
} satisfies Record<string, Interpolation<Theme>>;
