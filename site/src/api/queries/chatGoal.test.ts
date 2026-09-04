import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { chatGoalActionUnavailableReason, isChatBusyStatus } from "./chatGoal";

describe("isChatBusyStatus", () => {
	it.each([
		{ status: "running", busy: true },
		{ status: "interrupting", busy: true },
		{ status: "requires_action", busy: true },
		{ status: "waiting", busy: false },
		{ status: "error", busy: false },
		{ status: null, busy: false },
		{ status: undefined, busy: false },
	] as const)("$status is busy=$busy", ({ status, busy }) => {
		expect(isChatBusyStatus(status)).toBe(busy);
	});
});

describe("chatGoalActionUnavailableReason", () => {
	const busyReason =
		"The chat is busy. Resume becomes available when it is idle.";
	const planModeReason = "Turn off plan mode to resume the goal.";
	const noModelReason = "No AI model is available to resume the goal.";

	it.each<{
		name: string;
		action: "pause" | "resume" | "complete" | "clear";
		chatStatus?: TypesGen.ChatStatus | null;
		hasQueuedInput?: boolean;
		planModeEnabled?: boolean;
		hasModelOptions?: boolean;
		expected: string | undefined;
	}>([
		{
			name: "resume on idle chat is available",
			action: "resume",
			chatStatus: "waiting",
			expected: undefined,
		},
		{
			name: "resume on errored chat without queue is available",
			action: "resume",
			chatStatus: "error",
			expected: undefined,
		},
		{
			name: "resume while running is unavailable",
			action: "resume",
			chatStatus: "running",
			expected: busyReason,
		},
		{
			name: "resume while interrupting is unavailable",
			action: "resume",
			chatStatus: "interrupting",
			expected: busyReason,
		},
		{
			name: "resume while requiring action is unavailable",
			action: "resume",
			chatStatus: "requires_action",
			expected: busyReason,
		},
		{
			name: "resume with queued input is unavailable",
			action: "resume",
			chatStatus: "error",
			hasQueuedInput: true,
			expected: busyReason,
		},
		{
			name: "resume in plan mode is unavailable",
			action: "resume",
			chatStatus: "waiting",
			planModeEnabled: true,
			expected: planModeReason,
		},
		{
			name: "busy wins over plan mode",
			action: "resume",
			chatStatus: "running",
			planModeEnabled: true,
			expected: busyReason,
		},
		{
			name: "resume without a usable model is unavailable",
			action: "resume",
			chatStatus: "waiting",
			hasModelOptions: false,
			expected: noModelReason,
		},
		{
			name: "resume with models available is available",
			action: "resume",
			chatStatus: "waiting",
			hasModelOptions: true,
			expected: undefined,
		},
		{
			name: "pause is never gated by chat status",
			action: "pause",
			chatStatus: "running",
			expected: undefined,
		},
		{
			name: "complete is never gated by chat status",
			action: "complete",
			chatStatus: "running",
			expected: undefined,
		},
		{
			name: "clear is never gated by chat status",
			action: "clear",
			chatStatus: "running",
			hasQueuedInput: true,
			planModeEnabled: true,
			expected: undefined,
		},
	])("$name", ({
		action,
		chatStatus,
		hasQueuedInput,
		planModeEnabled,
		hasModelOptions,
		expected,
	}) => {
		expect(
			chatGoalActionUnavailableReason(action, {
				chatStatus,
				hasQueuedInput,
				planModeEnabled,
				hasModelOptions,
			}),
		).toBe(expected);
	});
});
