import {
	type ChatGoalAction,
	chatGoalActionAllowed,
	chatGoalActionUnavailableReason,
} from "#/api/queries/chatGoal";
import type * as TypesGen from "#/api/typesGenerated";

/** @internal Exported for testing. */
export const runGoalAction = async (params: {
	agentId: string | undefined;
	goal: TypesGen.ChatGoal | undefined;
	action: ChatGoalAction;
	completionSummary?: string;
	updateGoal: (variables: {
		chatId: string;
		mutation: TypesGen.ChatGoalUpdateRequest;
	}) => Promise<unknown>;
	liveChatStatus?: TypesGen.ChatStatus | null;
	hasQueuedInput?: boolean;
	planModeEnabled?: boolean;
	hasModelOptions?: boolean;
	onMissingGoal?: () => void;
	onActionUnavailable?: (reason: string) => void;
	onPausedRunningGoal?: () => void;
}): Promise<void> => {
	const {
		agentId,
		goal,
		action,
		completionSummary,
		updateGoal,
		liveChatStatus,
		hasQueuedInput,
		planModeEnabled,
		hasModelOptions,
		onMissingGoal,
		onActionUnavailable,
		onPausedRunningGoal,
	} = params;
	if (!agentId) {
		return;
	}
	if (!goal?.id || !chatGoalActionAllowed(goal, action)) {
		onMissingGoal?.();
		return;
	}
	// Mirror the server-side admission rules so the UI explains a
	// rejection instead of surfacing a 409.
	const unavailableReason = chatGoalActionUnavailableReason(action, {
		chatStatus: liveChatStatus,
		hasQueuedInput,
		planModeEnabled,
		hasModelOptions,
	});
	if (unavailableReason) {
		onActionUnavailable?.(unavailableReason);
		return;
	}
	await updateGoal({
		chatId: agentId,
		mutation: {
			action,
			goal_id: goal.id,
			completion_summary: completionSummary,
		},
	});
	if (action === "pause" && liveChatStatus === "running") {
		onPausedRunningGoal?.();
	}
};
