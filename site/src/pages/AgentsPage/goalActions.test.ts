import { describe, expect, it, vi } from "vitest";
import type { ChatGoal } from "#/api/typesGenerated";
import { MockChatGoal } from "#/testHelpers/chatEntities";
import { runGoalAction } from "./goalActions";

describe("runGoalAction", () => {
	const makeGoal = (status: ChatGoal["status"]): ChatGoal => ({
		...MockChatGoal,
		status,
	});

	it.each([
		{ status: "active", action: "clear", completionSummary: undefined },
		{ status: "paused", action: "clear", completionSummary: undefined },
		{ status: "paused", action: "resume", completionSummary: undefined },
		{ status: "blocked", action: "resume", completionSummary: undefined },
		{ status: "blocked", action: "clear", completionSummary: undefined },
		{ status: "complete", action: "clear", completionSummary: undefined },
		{
			status: "active",
			action: "complete",
			completionSummary: "Shipped and verified.",
		},
	] as const)(
		"sends $action mutations for $status goals",
		async ({ status, action, completionSummary }) => {
			const updateGoal = vi.fn(async () => undefined);

			await runGoalAction({
				agentId: "chat-1",
				goal: makeGoal(status),
				action,
				completionSummary,
				updateGoal,
			});

			expect(updateGoal).toHaveBeenCalledWith({
				chatId: "chat-1",
				mutation: {
					action,
					goal_id: "goal-1",
					completion_summary: completionSummary,
				},
			});
		},
	);

	it.each([
		{ liveChatStatus: "running", notified: true },
		{ liveChatStatus: "waiting", notified: false },
	] as const)(
		"notifies on pause only for running goals (status $liveChatStatus)",
		async ({ liveChatStatus, notified }) => {
			const updateGoal = vi.fn(async () => undefined);
			const onPausedRunningGoal = vi.fn();

			await runGoalAction({
				agentId: "chat-1",
				goal: makeGoal("active"),
				action: "pause",
				updateGoal,
				liveChatStatus,
				onPausedRunningGoal,
			});

			expect(onPausedRunningGoal).toHaveBeenCalledTimes(notified ? 1 : 0);
		},
	);

	it("does not send lifecycle mutations without a current goal", async () => {
		const updateGoal = vi.fn(async () => undefined);
		const onMissingGoal = vi.fn();

		await runGoalAction({
			agentId: "chat-1",
			goal: undefined,
			action: "clear",
			updateGoal,
			onMissingGoal,
		});

		expect(updateGoal).not.toHaveBeenCalled();
		expect(onMissingGoal).toHaveBeenCalledOnce();
	});

	it.each([
		{
			name: "running chat",
			liveChatStatus: "running",
			hasQueuedInput: false,
			planModeEnabled: false,
		},
		{
			name: "interrupting chat",
			liveChatStatus: "interrupting",
			hasQueuedInput: false,
			planModeEnabled: false,
		},
		{
			name: "queued input",
			liveChatStatus: "error",
			hasQueuedInput: true,
			planModeEnabled: false,
		},
		{
			name: "plan mode",
			liveChatStatus: "waiting",
			hasQueuedInput: false,
			planModeEnabled: true,
		},
	] as const)(
		"does not resume with $name",
		async ({ liveChatStatus, hasQueuedInput, planModeEnabled }) => {
			const updateGoal = vi.fn(async () => undefined);
			const onActionUnavailable = vi.fn();

			await runGoalAction({
				agentId: "chat-1",
				goal: makeGoal("paused"),
				action: "resume",
				updateGoal,
				liveChatStatus,
				hasQueuedInput,
				planModeEnabled,
				onActionUnavailable,
			});

			expect(updateGoal).not.toHaveBeenCalled();
			expect(onActionUnavailable).toHaveBeenCalledOnce();
			expect(onActionUnavailable).toHaveBeenCalledWith(expect.any(String));
		},
	);

	it("resumes on an idle chat", async () => {
		const updateGoal = vi.fn(async () => undefined);
		const onActionUnavailable = vi.fn();

		await runGoalAction({
			agentId: "chat-1",
			goal: makeGoal("paused"),
			action: "resume",
			updateGoal,
			liveChatStatus: "waiting",
			hasQueuedInput: false,
			planModeEnabled: false,
			onActionUnavailable,
		});

		expect(onActionUnavailable).not.toHaveBeenCalled();
		expect(updateGoal).toHaveBeenCalledWith({
			chatId: "chat-1",
			mutation: {
				action: "resume",
				goal_id: "goal-1",
				completion_summary: undefined,
			},
		});
	});
});
