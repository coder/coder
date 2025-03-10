import Tooltip from "@mui/material/Tooltip";
import type { Workspace, WorkspaceBuildParameter } from "api/typesGenerated";
import { TopbarButton } from "components/FullPageLayout/Topbar";
import {
	BanIcon,
	CirclePlayIcon,
	CircleStopIcon,
	CloudIcon,
	PowerIcon,
	RotateCcwIcon,
	StarIcon,
	StarOffIcon,
} from "lucide-react";
import type { FC } from "react";
import { BuildParametersPopover } from "./BuildParametersPopover";

export interface ActionButtonProps {
	loading?: boolean;
	handleAction: (buildParameters?: WorkspaceBuildParameter[]) => void;
	disabled?: boolean;
	tooltipText?: string;
}

export const UpdateButton: FC<ActionButtonProps> = ({
	handleAction,
	loading,
}) => {
	return (
		<TopbarButton
			disabled={loading}
			data-testid="workspace-update-button"
			onClick={() => handleAction()}
		>
			<CloudIcon />
			{loading ? <>Updating&hellip;</> : <>Update&hellip;</>}
		</TopbarButton>
	);
};

export const ActivateButton: FC<ActionButtonProps> = ({
	handleAction,
	loading,
}) => {
	return (
		<TopbarButton disabled={loading} onClick={() => handleAction()}>
			<PowerIcon />
			{loading ? <>Activating&hellip;</> : "Activate"}
		</TopbarButton>
	);
};

interface ActionButtonPropsWithWorkspace extends ActionButtonProps {
	workspace: Workspace;
}

export const StartButton: FC<ActionButtonPropsWithWorkspace> = ({
	handleAction,
	workspace,
	loading,
	disabled,
	tooltipText,
}) => {
	let mainButton = (
		<TopbarButton onClick={() => handleAction()} disabled={disabled || loading}>
			<CirclePlayIcon />
			{loading ? <>Starting&hellip;</> : "Start"}
		</TopbarButton>
	);

	if (tooltipText) {
		mainButton = <Tooltip title={tooltipText}>{mainButton}</Tooltip>;
	}

	return (
		<div className="flex gap-1 items-center">
			{mainButton}
			<BuildParametersPopover
				label="Start with build parameters"
				workspace={workspace}
				disabled={loading}
				onSubmit={handleAction}
			/>
		</div>
	);
};

export const UpdateAndStartButton: FC<ActionButtonProps> = ({
	handleAction,
}) => {
	return (
		<Tooltip title="This template requires automatic updates on workspace startup. Contact your administrator if you want to preserve the template version.">
			<TopbarButton onClick={() => handleAction()}>
				<CirclePlayIcon />
				Update and start&hellip;
			</TopbarButton>
		</Tooltip>
	);
};

export const StopButton: FC<ActionButtonProps> = ({
	handleAction,
	loading,
}) => {
	return (
		<TopbarButton
			disabled={loading}
			onClick={() => handleAction()}
			data-testid="workspace-stop-button"
		>
			<CircleStopIcon />
			{loading ? <>Stopping&hellip;</> : "Stop"}
		</TopbarButton>
	);
};

export const RestartButton: FC<ActionButtonPropsWithWorkspace> = ({
	handleAction,
	loading,
	workspace,
}) => {
	return (
		<div className="flex gap-1 items-center">
			<TopbarButton
				onClick={() => handleAction()}
				data-testid="workspace-restart-button"
				disabled={loading}
			>
				<RotateCcwIcon />
				{loading ? <>Restarting&hellip;</> : <>Restart&hellip;</>}
			</TopbarButton>
			<BuildParametersPopover
				label="Restart with build parameters"
				workspace={workspace}
				disabled={loading}
				onSubmit={handleAction}
			/>
		</div>
	);
};

export const UpdateAndRestartButton: FC<ActionButtonProps> = ({
	handleAction,
}) => {
	return (
		<Tooltip title="This template requires automatic updates on workspace startup. Contact your administrator if you want to preserve the template version.">
			<TopbarButton onClick={() => handleAction()}>
				<RotateCcwIcon />
				Update and restart&hellip;
			</TopbarButton>
		</Tooltip>
	);
};

export const CancelButton: FC<ActionButtonProps> = ({ handleAction }) => {
	return (
		<TopbarButton onClick={() => handleAction()}>
			<BanIcon />
			Cancel
		</TopbarButton>
	);
};

interface DisabledButtonProps {
	label: string;
}

export const DisabledButton: FC<DisabledButtonProps> = ({ label }) => {
	return (
		<TopbarButton disabled>
			<BanIcon />
			{label}
		</TopbarButton>
	);
};

interface FavoriteButtonProps {
	onToggle: (workspaceID: string) => void;
	workspaceID: string;
	isFavorite: boolean;
}

export const FavoriteButton: FC<FavoriteButtonProps> = ({
	onToggle,
	workspaceID,
	isFavorite,
}) => {
	return (
		<TopbarButton onClick={() => onToggle(workspaceID)}>
			{isFavorite ? <StarOffIcon /> : <StarIcon />}
			{isFavorite ? "Unfavorite" : "Favorite"}
		</TopbarButton>
	);
};
