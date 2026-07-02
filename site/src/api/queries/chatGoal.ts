import type * as TypesGen from "#/api/typesGenerated";

export type ChatGoalAction = Exclude<TypesGen.ChatGoalMutationAction, "set">;
export type CurrentChatGoalStatus = Extract<
	TypesGen.ChatGoalStatus,
	"active" | "paused" | "complete"
>;

export const isCurrentChatGoalStatus = (
	status: TypesGen.ChatGoalStatus,
): status is CurrentChatGoalStatus =>
	status === "active" || status === "paused" || status === "complete";

export const currentChatGoal = (
	goal: TypesGen.ChatGoal | undefined,
): TypesGen.ChatGoal | undefined =>
	goal && isCurrentChatGoalStatus(goal.status) ? goal : undefined;

const CHAT_GOAL_ACTIONS_BY_STATUS = {
	active: ["pause", "complete", "clear"],
	paused: ["resume", "clear"],
	complete: ["clear"],
} as const satisfies Record<CurrentChatGoalStatus, readonly ChatGoalAction[]>;

export const chatGoalActionsForStatus = (
	status: CurrentChatGoalStatus,
): readonly ChatGoalAction[] => CHAT_GOAL_ACTIONS_BY_STATUS[status];

export const chatGoalActionAllowed = (
	goal: TypesGen.ChatGoal,
	action: ChatGoalAction,
): boolean =>
	isCurrentChatGoalStatus(goal.status) &&
	chatGoalActionsForStatus(goal.status).includes(action);
