import IconButton from "@mui/material/IconButton";
import Link, { type LinkProps } from "@mui/material/Link";
import Tooltip from "@mui/material/Tooltip";
import { visuallyHidden } from "@mui/utils";
import { getErrorMessage } from "api/errors";
import {
	updateDeadline,
	workspaceByOwnerAndNameKey,
} from "api/queries/workspaces";
import type { Template, Workspace } from "api/typesGenerated";
import { TopbarData, TopbarIcon } from "components/FullPageLayout/Topbar";
import { displayError, displaySuccess } from "components/GlobalSnackbar/utils";
import dayjs, { type Dayjs } from "dayjs";
import { useTime } from "hooks/useTime";
import { ClockIcon, MinusIcon, PlusIcon } from "lucide-react";
import { getWorkspaceActivityStatus } from "modules/workspaces/activity";
import { type FC, forwardRef, type ReactNode, useRef, useState } from "react";
import { useMutation, useQueryClient } from "react-query";
import { Link as RouterLink } from "react-router";
import { cn } from "utils/cn";
import {
	autostartDisplay,
	autostopDisplay,
	getDeadline,
	getMaxDeadline,
	getMaxDeadlineChange,
	getMinDeadline,
} from "utils/schedule";
import { isWorkspaceOn } from "utils/workspace";

interface WorkspaceScheduleContainerProps {
	children?: ReactNode;
	onClickIcon?: () => void;
}

const WorkspaceScheduleContainer: FC<WorkspaceScheduleContainerProps> = ({
	children,
	onClickIcon,
}) => {
	const icon = (
		<TopbarIcon>
			<ClockIcon aria-label="Schedule" className="size-icon-sm" />
		</TopbarIcon>
	);

	return (
		<TopbarData>
			<Tooltip title="Schedule">
				{onClickIcon ? (
					<button
						type="button"
						data-testid="schedule-icon-button"
						onClick={onClickIcon}
						className={
							"flex items-center bg-transparent border-0 p-0 font-inherit cursor-pointer"
						}
					>
						{icon}
					</button>
				) : (
					icon
				)}
			</Tooltip>
			{children}
		</TopbarData>
	);
};

interface WorkspaceScheduleControlsProps {
	workspace: Workspace;
	template: Template;
	canUpdateSchedule: boolean;
}

export const WorkspaceScheduleControls: FC<WorkspaceScheduleControlsProps> = ({
	workspace,
	template,
	canUpdateSchedule,
}) => {
	if (!shouldDisplayScheduleControls(workspace)) {
		return null;
	}

	return (
		<div
			className="flex items-center gap-12 tabular-nums"
			data-testid="schedule-controls"
		>
			{isWorkspaceOn(workspace) ? (
				<AutostopDisplay
					workspace={workspace}
					template={template}
					canUpdateSchedule={canUpdateSchedule}
				/>
			) : (
				<WorkspaceScheduleContainer>
					<ScheduleSettingsLink>
						Starts at {autostartDisplay(workspace.autostart_schedule)}
					</ScheduleSettingsLink>
				</WorkspaceScheduleContainer>
			)}
		</div>
	);
};

interface AutostopDisplayProps {
	workspace: Workspace;
	template: Template;
	canUpdateSchedule: boolean;
}

const AutostopDisplay: FC<AutostopDisplayProps> = ({
	workspace,
	template,
	canUpdateSchedule,
}) => {
	const queryClient = useQueryClient();
	const deadline = getDeadline(workspace);
	const maxDeadlineDecrease = getMaxDeadlineChange(deadline, getMinDeadline());
	const maxDeadlineIncrease = getMaxDeadlineChange(
		getMaxDeadline(workspace),
		deadline,
	);
	const deadlinePlusEnabled = maxDeadlineIncrease >= 1;
	const deadlineMinusEnabled = maxDeadlineDecrease >= 1;
	const deadlineUpdateTimeout = useRef<number>(undefined);
	const lastStableDeadline = useRef<Dayjs>(deadline);

	const updateWorkspaceDeadlineQueryData = (deadline: Dayjs) => {
		queryClient.setQueryData(
			workspaceByOwnerAndNameKey(workspace.owner_name, workspace.name),
			{
				...workspace,
				latest_build: {
					...workspace.latest_build,
					deadline: deadline.toISOString(),
				},
			},
		);
	};

	const updateDeadlineMutation = useMutation({
		...updateDeadline(workspace),
		onSuccess: (_, updatedDeadline) => {
			displaySuccess("Workspace shutdown time has been successfully updated.");
			lastStableDeadline.current = updatedDeadline;
		},
		onError: (error) => {
			displayError(
				getErrorMessage(
					error,
					"We couldn't update your workspace shutdown time. Please try again.",
				),
			);
			updateWorkspaceDeadlineQueryData(lastStableDeadline.current);
		},
	});

	const handleDeadlineChange = (newDeadline: Dayjs) => {
		clearTimeout(deadlineUpdateTimeout.current);
		// Optimistic update
		updateWorkspaceDeadlineQueryData(newDeadline);
		deadlineUpdateTimeout.current = window.setTimeout(() => {
			updateDeadlineMutation.mutate(newDeadline);
		}, 500);
	};

	const activityStatus = useTime(() => getWorkspaceActivityStatus(workspace));
	const { message, tooltip, danger } = autostopDisplay(
		workspace,
		activityStatus,
		template,
	);

	const [showControlsAnyway, setShowControlsAnyway] = useState(false);
	let onClickScheduleIcon: (() => void) | undefined;

	if (activityStatus === "connected") {
		onClickScheduleIcon = () => setShowControlsAnyway((it) => !it);

		const now = dayjs();
		const noRequiredStopSoon =
			!workspace.latest_build.max_deadline ||
			dayjs(workspace.latest_build.max_deadline).isAfter(now.add(2, "hour"));

		// User has shown controls manually, or we should warn about a nearby required stop
		if (!showControlsAnyway && noRequiredStopSoon) {
			return <WorkspaceScheduleContainer onClickIcon={onClickScheduleIcon} />;
		}
	}

	const display = (
		<ScheduleSettingsLink
			data-testid="schedule-controls-autostop"
			className={cn([danger && "!text-content-destructive"])}
		>
			{message}
		</ScheduleSettingsLink>
	);

	const controls = canUpdateSchedule && canEditDeadline(workspace) && (
		<div className="flex items-center gap-4">
			<Tooltip title="Subtract 1 hour from deadline">
				<IconButton
					disabled={!deadlineMinusEnabled}
					size="small"
					className={
						"border border-solid border-zinc-700 rounded-sm w-5 h-5 [&>svg]:size-icon-xs"
					}
					onClick={() => {
						handleDeadlineChange(deadline.subtract(1, "h"));
					}}
				>
					<MinusIcon className="size-icon-xs" />
					<span style={visuallyHidden}>Subtract 1 hour</span>
				</IconButton>
			</Tooltip>
			<Tooltip title="Add 1 hour to deadline">
				<IconButton
					disabled={!deadlinePlusEnabled}
					size="small"
					className={
						"border border-solid border-zinc-700 rounded-sm w-5 h-5 [&>svg]:size-icon-xs"
					}
					onClick={() => {
						handleDeadlineChange(deadline.add(1, "h"));
					}}
				>
					<PlusIcon className="size-icon-xs" />
					<span style={visuallyHidden}>Add 1 hour</span>
				</IconButton>
			</Tooltip>
		</div>
	);

	if (tooltip) {
		return (
			<WorkspaceScheduleContainer onClickIcon={onClickScheduleIcon}>
				<Tooltip title={tooltip}>{display}</Tooltip>
				{controls}
			</WorkspaceScheduleContainer>
		);
	}

	return (
		<WorkspaceScheduleContainer onClickIcon={onClickScheduleIcon}>
			{display}
			{controls}
		</WorkspaceScheduleContainer>
	);
};

const ScheduleSettingsLink = forwardRef<HTMLAnchorElement, LinkProps>(
	(props, ref) => {
		return (
			<Link
				ref={ref}
				component={RouterLink}
				to="settings/schedule"
				className="text-inherit [&:first-letter]:uppercase"
				{...props}
			/>
		);
	},
);

const hasDeadline = (workspace: Workspace): boolean => {
	return Boolean(workspace.latest_build.deadline);
};

const hasAutoStart = (workspace: Workspace): boolean => {
	return Boolean(workspace.autostart_schedule);
};

const canEditDeadline = (workspace: Workspace): boolean => {
	return isWorkspaceOn(workspace) && hasDeadline(workspace);
};

const shouldDisplayScheduleControls = (workspace: Workspace): boolean => {
	const willAutoStop = isWorkspaceOn(workspace) && hasDeadline(workspace);
	const willAutoStart = !isWorkspaceOn(workspace) && hasAutoStart(workspace);
	return willAutoStop || willAutoStart;
};
