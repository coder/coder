import type { Interpolation, Theme } from "@emotion/react";
import Link from "@mui/material/Link";
import type {
	WorkspaceAgent,
	WorkspaceAgentDevcontainer,
} from "api/typesGenerated";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import {
	HelpTooltip,
	HelpTooltipContent,
	HelpTooltipText,
	HelpTooltipTitle,
	HelpTooltipTrigger,
} from "components/HelpTooltip/HelpTooltip";
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "components/Tooltip/Tooltip";
import { TriangleAlertIcon } from "lucide-react";
import type { FC } from "react";

// If we think in the agent status and lifecycle into a single enum/state I'd
// say we would have: connecting, timeout, disconnected, connected:created,
// connected:starting, connected:start_timeout, connected:start_error,
// connected:ready, connected:shutting_down, connected:shutdown_timeout,
// connected:shutdown_error, connected:off.

const ReadyLifecycle: FC = () => {
	return (
		<div
			role="status"
			data-testid="agent-status-ready"
			aria-label="Ready"
			css={[styles.status, styles.connected]}
		/>
	);
};

const StartingLifecycle: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<div
					role="status"
					aria-label="Starting..."
					css={[styles.status, styles.connecting]}
				/>
			</TooltipTrigger>
			<TooltipContent side="bottom">Starting...</TooltipContent>
		</Tooltip>
	);
};

interface AgentStatusProps {
	agent: WorkspaceAgent;
}

interface SubAgentStatusProps {
	agent?: WorkspaceAgent;
}

interface DevcontainerStatusProps {
	devcontainer: WorkspaceAgentDevcontainer;
	parentAgent: WorkspaceAgent;
	agent?: WorkspaceAgent;
}

const StartTimeoutLifecycle: FC<AgentStatusProps> = ({ agent }) => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger asChild role="status" aria-label="Agent timeout">
				<TriangleAlertIcon css={styles.timeoutWarning} />
			</HelpTooltipTrigger>

			<HelpTooltipContent>
				<HelpTooltipTitle>Agent is taking too long to start</HelpTooltipTitle>
				<HelpTooltipText>
					We noticed this agent is taking longer than expected to start.{" "}
					<Link
						target="_blank"
						rel="noreferrer"
						href={agent.troubleshooting_url}
					>
						Troubleshoot
					</Link>
					.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

const StartErrorLifecycle: FC<AgentStatusProps> = ({ agent }) => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger asChild role="status" aria-label="Start error">
				<TriangleAlertIcon css={styles.errorWarning} />
			</HelpTooltipTrigger>
			<HelpTooltipContent>
				<HelpTooltipTitle>Error starting the agent</HelpTooltipTitle>
				<HelpTooltipText>
					Something went wrong during the agent startup.{" "}
					<Link
						target="_blank"
						rel="noreferrer"
						href={agent.troubleshooting_url}
					>
						Troubleshoot
					</Link>
					.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

const ShuttingDownLifecycle: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<div
					role="status"
					aria-label="Stopping..."
					css={[styles.status, styles.connecting]}
				/>
			</TooltipTrigger>
			<TooltipContent side="bottom">Stopping...</TooltipContent>
		</Tooltip>
	);
};

const ShutdownTimeoutLifecycle: FC<AgentStatusProps> = ({ agent }) => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger asChild role="status" aria-label="Stop timeout">
				<TriangleAlertIcon css={styles.timeoutWarning} />
			</HelpTooltipTrigger>
			<HelpTooltipContent>
				<HelpTooltipTitle>Agent is taking too long to stop</HelpTooltipTitle>
				<HelpTooltipText>
					We noticed this agent is taking longer than expected to stop.{" "}
					<Link
						target="_blank"
						rel="noreferrer"
						href={agent.troubleshooting_url}
					>
						Troubleshoot
					</Link>
					.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

const ShutdownErrorLifecycle: FC<AgentStatusProps> = ({ agent }) => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger asChild role="status" aria-label="Stop error">
				<TriangleAlertIcon css={styles.errorWarning} />
			</HelpTooltipTrigger>
			<HelpTooltipContent>
				<HelpTooltipTitle>Error stopping the agent</HelpTooltipTitle>
				<HelpTooltipText>
					Something went wrong while trying to stop the agent.{" "}
					<Link
						target="_blank"
						rel="noreferrer"
						href={agent.troubleshooting_url}
					>
						Troubleshoot
					</Link>
					.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

const OffLifecycle: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<div
					role="status"
					aria-label="Stopped"
					css={[styles.status, styles.disconnected]}
				/>
			</TooltipTrigger>
			<TooltipContent side="bottom">Stopped</TooltipContent>
		</Tooltip>
	);
};

const ConnectedStatus: FC<AgentStatusProps> = ({ agent }) => {
	// This is to support legacy agents that do not support
	// reporting the lifecycle_state field.
	if (agent.scripts.length === 0) {
		return <ReadyLifecycle />;
	}
	return (
		<ChooseOne>
			<Cond condition={agent.lifecycle_state === "ready"}>
				<ReadyLifecycle />
			</Cond>
			<Cond condition={agent.lifecycle_state === "start_timeout"}>
				<StartTimeoutLifecycle agent={agent} />
			</Cond>
			<Cond condition={agent.lifecycle_state === "start_error"}>
				<StartErrorLifecycle agent={agent} />
			</Cond>
			<Cond condition={agent.lifecycle_state === "shutting_down"}>
				<ShuttingDownLifecycle />
			</Cond>
			<Cond condition={agent.lifecycle_state === "shutdown_timeout"}>
				<ShutdownTimeoutLifecycle agent={agent} />
			</Cond>
			<Cond condition={agent.lifecycle_state === "shutdown_error"}>
				<ShutdownErrorLifecycle agent={agent} />
			</Cond>
			<Cond condition={agent.lifecycle_state === "off"}>
				<OffLifecycle />
			</Cond>
			<Cond>
				<StartingLifecycle />
			</Cond>
		</ChooseOne>
	);
};

const DisconnectedStatus: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<div
					role="status"
					aria-label="Disconnected"
					css={[styles.status, styles.disconnected]}
				/>
			</TooltipTrigger>
			<TooltipContent side="bottom">Disconnected</TooltipContent>
		</Tooltip>
	);
};

const ConnectingStatus: FC = () => {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<div
					role="status"
					aria-label="Connecting..."
					css={[styles.status, styles.connecting]}
				/>
			</TooltipTrigger>
			<TooltipContent side="bottom">Connecting...</TooltipContent>
		</Tooltip>
	);
};

const TimeoutStatus: FC<AgentStatusProps> = ({ agent }) => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger asChild role="status" aria-label="Timeout">
				<TriangleAlertIcon css={styles.timeoutWarning} />
			</HelpTooltipTrigger>
			<HelpTooltipContent>
				<HelpTooltipTitle>Agent is taking too long to connect</HelpTooltipTitle>
				<HelpTooltipText>
					We noticed this agent is taking longer than expected to connect.{" "}
					<Link
						target="_blank"
						rel="noreferrer"
						href={agent.troubleshooting_url}
					>
						Troubleshoot
					</Link>
					.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

export const AgentStatus: FC<AgentStatusProps> = ({ agent }) => {
	return (
		<ChooseOne>
			<Cond condition={agent.status === "connected"}>
				<ConnectedStatus agent={agent} />
			</Cond>
			<Cond condition={agent.status === "disconnected"}>
				<DisconnectedStatus />
			</Cond>
			<Cond condition={agent.status === "timeout"}>
				<TimeoutStatus agent={agent} />
			</Cond>
			<Cond>
				<ConnectingStatus />
			</Cond>
		</ChooseOne>
	);
};

const SubAgentStatus: FC<SubAgentStatusProps> = ({ agent }) => {
	if (!agent) {
		return <DisconnectedStatus />;
	}
	return (
		<ChooseOne>
			<Cond condition={agent.status === "connected"}>
				<ConnectedStatus agent={agent} />
			</Cond>
			<Cond condition={agent.status === "disconnected"}>
				<DisconnectedStatus />
			</Cond>
			<Cond condition={agent.status === "timeout"}>
				<TimeoutStatus agent={agent} />
			</Cond>
			<Cond>
				<ConnectingStatus />
			</Cond>
		</ChooseOne>
	);
};

const DevcontainerStartError: FC<AgentStatusProps> = ({ agent }) => {
	return (
		<HelpTooltip>
			<HelpTooltipTrigger asChild role="status" aria-label="Start error">
				<TriangleAlertIcon css={styles.errorWarning} />
			</HelpTooltipTrigger>
			<HelpTooltipContent>
				<HelpTooltipTitle>
					Error starting the devcontainer agent
				</HelpTooltipTitle>
				<HelpTooltipText>
					Something went wrong during the devcontainer agent startup.{" "}
					<Link
						target="_blank"
						rel="noreferrer"
						href={agent.troubleshooting_url}
					>
						Troubleshoot
					</Link>
					.
				</HelpTooltipText>
			</HelpTooltipContent>
		</HelpTooltip>
	);
};

export const DevcontainerStatus: FC<DevcontainerStatusProps> = ({
	devcontainer,
	parentAgent,
	agent,
}) => {
	if (devcontainer.error) {
		// When a dev container has an 'error' associated with it,
		// then we won't have an agent associated with it. This is
		// why we use the parent agent instead of the sub agent.
		return <DevcontainerStartError agent={parentAgent} />;
	}

	return <SubAgentStatus agent={agent} />;
};

const styles = {
	status: {
		width: 6,
		height: 6,
		borderRadius: "100%",
		flexShrink: 0,
	},

	connected: (theme) => ({
		backgroundColor: theme.palette.success.light,
		boxShadow: `0 0 12px 0 ${theme.palette.success.light}`,
	}),

	disconnected: (theme) => ({
		backgroundColor: theme.palette.text.secondary,
	}),

	"@keyframes pulse": {
		"0%": {
			opacity: 1,
		},
		"50%": {
			opacity: 0.4,
		},
		"100%": {
			opacity: 1,
		},
	},

	connecting: (theme) => ({
		backgroundColor: theme.palette.info.light,
		animation: "$pulse 1.5s 0.5s ease-in-out forwards infinite",
	}),

	timeoutWarning: (theme) => ({
		color: theme.palette.warning.light,
		width: 14,
		height: 14,
		position: "relative",
	}),

	errorWarning: (theme) => ({
		color: theme.palette.error.main,
		width: 14,
		height: 14,
		position: "relative",
	}),
} satisfies Record<string, Interpolation<Theme>>;
