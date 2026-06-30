import {
	CheckIcon,
	CirclePauseIcon,
	CirclePlayIcon,
	type LucideIcon,
	TargetIcon,
	Trash2Icon,
} from "lucide-react";
import type { ComponentProps, FC } from "react";
import {
	type ChatGoalAction,
	type CurrentChatGoalStatus,
	chatGoalActionsForStatus,
	isCurrentChatGoalStatus,
} from "#/api/queries/chatGoal";
import type * as TypesGen from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { relativeTime, shortRelativeTime } from "#/utils/time";

type ChatGoalBannerProps = {
	goal: TypesGen.ChatGoal | undefined;
	canMutateGoal?: boolean;
	isActionPending?: boolean;
	isActionDisabled?: boolean;
	onAction: (action: ChatGoalAction) => Promise<void> | void;
};

type GoalStatusUI = {
	label: string;
	variant: ComponentProps<typeof Badge>["variant"];
};

const GOAL_STATUS_UI = {
	active: { label: "Pursuing goal", variant: "info" },
	paused: { label: "Goal paused", variant: "warning" },
	complete: { label: "Goal complete", variant: "green" },
} satisfies Record<CurrentChatGoalStatus, GoalStatusUI>;

type GoalActionUI = {
	label: string;
	Icon: LucideIcon;
};

const GOAL_ACTION_UI = {
	pause: { label: "Pause", Icon: CirclePauseIcon },
	resume: { label: "Resume", Icon: CirclePlayIcon },
	complete: { label: "Complete", Icon: CheckIcon },
	clear: { label: "Clear", Icon: Trash2Icon },
} satisfies Record<ChatGoalAction, GoalActionUI>;

export const ChatGoalBanner: FC<ChatGoalBannerProps> = ({
	goal,
	canMutateGoal = false,
	isActionPending = false,
	isActionDisabled = false,
	onAction,
}) => {
	if (!goal || !isCurrentChatGoalStatus(goal.status)) {
		return null;
	}

	const statusUI = GOAL_STATUS_UI[goal.status];
	const actions = canMutateGoal ? chatGoalActionsForStatus(goal.status) : [];
	const disabled = isActionPending || isActionDisabled;
	const age = shortRelativeTime(goal.created_at);

	return (
		<section
			aria-label="Current goal"
			className="mx-auto mb-2 flex w-full max-w-3xl flex-col gap-2 rounded-lg border border-border-default bg-surface-secondary px-3 py-2 text-sm shadow-sm sm:flex-row sm:items-center sm:justify-between"
		>
			<div className="flex min-w-0 items-start gap-2 sm:items-center">
				<TargetIcon className="mt-0.5 size-icon-sm shrink-0 text-content-secondary sm:mt-0" />
				<div className="min-w-0 space-y-1">
					<div className="flex min-w-0 items-center gap-2">
						<Badge size="sm" variant={statusUI.variant}>
							{statusUI.label}
						</Badge>
						<span
							className="min-w-0 truncate text-content-primary"
							title={goal.objective}
						>
							{goal.objective}
						</span>
						<span
							className="hidden shrink-0 text-xs text-content-secondary sm:inline"
							title={`Started ${relativeTime(goal.created_at)}`}
						>
							{age}
						</span>
					</div>
					{goal.completion_summary ? (
						<p
							className="truncate text-xs text-content-secondary"
							title={goal.completion_summary}
						>
							Summary: {goal.completion_summary}
						</p>
					) : null}
				</div>
			</div>
			{actions.length > 0 ? (
				<div className="flex flex-wrap gap-1 sm:justify-end">
					{actions.map((action) => {
						const actionUI = GOAL_ACTION_UI[action];
						const Icon = actionUI.Icon;
						return (
							<Button
								key={action}
								size="xs"
								variant={action === "clear" ? "subtle" : "outline"}
								disabled={disabled}
								onClick={() => {
									void onAction(action);
								}}
							>
								<Icon />
								{actionUI.label}
							</Button>
						);
					})}
				</div>
			) : null}
		</section>
	);
};
