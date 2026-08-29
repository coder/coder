import type * as TypesGen from "#/api/typesGenerated";

export type ChatGoalAction = TypesGen.ChatGoalUpdateAction;
export type CurrentChatGoalStatus = Extract<
	TypesGen.ChatGoalStatus,
	"active" | "paused" | "blocked" | "complete"
>;

const CHAT_GOAL_ACTIONS_BY_STATUS = {
	active: ["pause", "complete", "clear"],
	paused: ["resume", "clear"],
	blocked: ["resume", "clear"],
	complete: ["clear"],
} as const satisfies Record<CurrentChatGoalStatus, readonly ChatGoalAction[]>;

export const isCurrentChatGoalStatus = (
	status: TypesGen.ChatGoalStatus,
): status is CurrentChatGoalStatus =>
	Object.hasOwn(CHAT_GOAL_ACTIONS_BY_STATUS, status);

export const currentChatGoal = (
	goal: TypesGen.ChatGoal | undefined,
): TypesGen.ChatGoal | undefined =>
	goal && isCurrentChatGoalStatus(goal.status) ? goal : undefined;

export const chatGoalActionsForStatus = (
	status: CurrentChatGoalStatus,
): readonly ChatGoalAction[] => CHAT_GOAL_ACTIONS_BY_STATUS[status];

export const chatGoalActionAllowed = (
	goal: TypesGen.ChatGoal,
	action: ChatGoalAction,
): boolean =>
	isCurrentChatGoalStatus(goal.status) &&
	chatGoalActionsForStatus(goal.status).includes(action);

export const isChatBusyStatus = (
	status: TypesGen.ChatStatus | null | undefined,
): boolean =>
	status === "running" ||
	status === "interrupting" ||
	status === "requires_action";

type ChatGoalActionContext = {
	chatStatus?: TypesGen.ChatStatus | null;
	hasQueuedInput?: boolean;
	planModeEnabled?: boolean;
	hasModelOptions?: boolean;
};

/**
 * Returns why a status-allowed goal action is unavailable right now, or
 * undefined when it can proceed. Resume starts a turn on the chat, so it
 * requires an idle chat with no queued input and plan mode off; the
 * server rejects it otherwise.
 */
export const chatGoalActionUnavailableReason = (
	action: ChatGoalAction,
	context: ChatGoalActionContext,
): string | undefined => {
	if (action !== "resume") {
		return undefined;
	}
	if (isChatBusyStatus(context.chatStatus) || context.hasQueuedInput) {
		return "The chat is busy. Resume becomes available when it is idle.";
	}
	if (context.planModeEnabled) {
		return "Turn off plan mode to resume the goal.";
	}
	// Resume starts a turn, which fails without a usable model.
	if (context.hasModelOptions === false) {
		return "No AI model is available to resume the goal.";
	}
	return undefined;
};
