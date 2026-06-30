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
			className="mx-auto mb-2 w-full max-w-3xl overflow-hidden rounded-xl border border-border-default/70 bg-surface-secondary/80 px-3 py-2.5 text-sm shadow-sm ring-1 ring-border-default/20"
		>
			<div className="flex min-w-0 items-start gap-2.5">
				<div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full border border-border-default bg-surface-primary/70">
					<TargetIcon className="size-4 text-content-secondary" />
				</div>
				<div className="min-w-0 flex-1 space-y-2">
					<div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
						<Badge size="sm" variant={statusUI.variant}>
							{statusUI.label}
						</Badge>
						<span
							className="text-xs text-content-secondary"
							title={`Started ${relativeTime(goal.created_at)}`}
						>
							Started {age}
						</span>
					</div>
					<p
						className="line-clamp-3 whitespace-pre-wrap text-sm leading-5 text-content-primary [overflow-wrap:anywhere]"
						title={goal.objective}
					>
						{goal.objective}
					</p>
					{goal.completion_summary ? (
						<p
							className="line-clamp-2 text-xs leading-5 text-content-secondary [overflow-wrap:anywhere]"
							title={goal.completion_summary}
						>
							Summary: {goal.completion_summary}
						</p>
					) : null}
					{actions.length > 0 ? (
						<div className="flex min-w-0 items-center justify-end gap-1.5 overflow-x-auto border-t border-border-default/60 pt-2">
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
				</div>
			</div>
		</section>
	);
};
