import {
	CheckIcon,
	CirclePauseIcon,
	CirclePlayIcon,
	type LucideIcon,
	TargetIcon,
	Trash2Icon,
} from "lucide-react";
import type { ComponentProps, FC } from "react";
import { Fragment } from "react";
import {
	type ChatGoalAction,
	type CurrentChatGoalStatus,
	chatGoalActionsForStatus,
	isCurrentChatGoalStatus,
} from "#/api/queries/chatGoal";
import type * as TypesGen from "#/api/typesGenerated";
import { ChatGoalMaxContinuationTurns } from "#/api/typesGenerated";
import { Badge } from "#/components/Badge/Badge";
import { Button } from "#/components/Button/Button";
import { relativeTime, shortRelativeTime } from "#/utils/time";

type ChatGoalBannerProps = {
	goal: TypesGen.ChatGoal | undefined;
	canMutateGoal?: boolean;
	isActionPending?: boolean;
	isActionDisabled?: boolean;
	/**
	 * Whether the chat is actively working (running, interrupting, or
	 * requires action). Drives the active-goal label so the banner never
	 * claims the agent is pursuing a goal while the chat sits idle.
	 */
	isChatWorking?: boolean;
	/**
	 * Per-action unavailability reasons. A present entry disables that
	 * action's button and explains why in its tooltip.
	 */
	actionUnavailableReasons?: Partial<Record<ChatGoalAction, string>>;
	onAction: (action: ChatGoalAction) => Promise<void> | void;
};

type GoalStatusUI = {
	label: string;
	variant: ComponentProps<typeof Badge>["variant"];
};

const GOAL_STATUS_UI = {
	active: { label: "Goal active", variant: "info" },
	paused: { label: "Goal paused", variant: "warning" },
	blocked: { label: "Goal blocked", variant: "warning" },
	complete: { label: "Goal complete", variant: "green" },
} satisfies Record<CurrentChatGoalStatus, GoalStatusUI>;

const PAUSED_REASON_LABELS: Record<TypesGen.ChatGoalPausedReason, string> = {
	user: "Paused by you",
	interrupt: "Paused by Stop",
	turn_limit: "Turn limit reached",
	usage_limit: "Usage limit reached",
	error: "Paused after an error",
};

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

const goalStatusDetail = (goal: TypesGen.ChatGoal): string | undefined => {
	if (goal.status === "active" && goal.continuation_count > 0) {
		return `Auto-continue ${goal.continuation_count}/${ChatGoalMaxContinuationTurns}`;
	}
	if (goal.status === "paused" && goal.paused_reason) {
		return PAUSED_REASON_LABELS[goal.paused_reason];
	}
	return undefined;
};

export const ChatGoalBanner: FC<ChatGoalBannerProps> = ({
	goal,
	canMutateGoal = false,
	isActionPending = false,
	isActionDisabled = false,
	isChatWorking = false,
	actionUnavailableReasons,
	onAction,
}) => {
	if (!goal || !isCurrentChatGoalStatus(goal.status)) {
		return null;
	}

	const statusUI =
		goal.status === "active" && isChatWorking
			? { label: "Pursuing goal", variant: "info" as const }
			: GOAL_STATUS_UI[goal.status];
	const actions = canMutateGoal ? chatGoalActionsForStatus(goal.status) : [];
	const disabled = isActionPending || isActionDisabled;
	const age = shortRelativeTime(goal.created_at);
	const statusDetail = goalStatusDetail(goal);

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
						{statusDetail ? (
							<span className="text-xs text-content-secondary">
								{statusDetail}
							</span>
						) : null}
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
					{goal.status === "blocked" && goal.blocked_reason ? (
						<p
							className="line-clamp-2 text-xs leading-5 text-content-warning [overflow-wrap:anywhere]"
							title={goal.blocked_reason}
						>
							Blocked: {goal.blocked_reason}
						</p>
					) : null}
					{actions.length > 0 ? (
						<div className="flex min-w-0 items-center justify-end gap-1.5 overflow-x-auto border-t border-border-default/60 pt-2">
							{actions.map((action) => {
								const actionUI = GOAL_ACTION_UI[action];
								const Icon = actionUI.Icon;
								const unavailableReason = actionUnavailableReasons?.[action];
								const isUnavailable = unavailableReason !== undefined;
								// An unavailable action stays focusable with aria-disabled
								// so keyboard and screen-reader users can reach the
								// explanation; a hard-disabled button cannot be focused.
								return (
									<Fragment key={action}>
										<Button
											size="xs"
											variant={action === "clear" ? "subtle" : "outline"}
											disabled={disabled}
											aria-disabled={isUnavailable || undefined}
											aria-describedby={
												isUnavailable
													? `goal-action-${action}-unavailable`
													: undefined
											}
											title={unavailableReason}
											className={
												isUnavailable
													? "text-content-disabled hover:text-content-disabled"
													: undefined
											}
											onClick={() => {
												if (isUnavailable) {
													return;
												}
												void onAction(action);
											}}
										>
											<Icon />
											{actionUI.label}
										</Button>
										{isUnavailable ? (
											<span
												id={`goal-action-${action}-unavailable`}
												className="sr-only"
											>
												{unavailableReason}
											</span>
										) : null}
									</Fragment>
								);
							})}
						</div>
					) : null}
				</div>
			</div>
		</section>
	);
};
