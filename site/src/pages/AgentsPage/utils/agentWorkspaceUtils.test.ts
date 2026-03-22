import { describe, expect, it } from "vitest";
import {
	isWorkspaceAutoCreated,
	resolveArchiveAndDeleteAction,
} from "./agentWorkspaceUtils";

describe("isWorkspaceAutoCreated", () => {
	it.each([
		{
			name: "workspace created after chat",
			workspace: "2026-01-01T00:00:05Z",
			chat: "2026-01-01T00:00:00Z",
			expected: true,
		},
		{
			name: "workspace created at same time as chat",
			workspace: "2026-01-01T12:00:00Z",
			chat: "2026-01-01T12:00:00Z",
			expected: true,
		},
		{
			name: "workspace created before chat",
			workspace: "2026-01-01T11:59:59Z",
			chat: "2026-01-01T12:00:00Z",
			expected: false,
		},
		{
			name: "sub-second precision difference",
			workspace: "2026-01-01T00:00:00.001Z",
			chat: "2026-01-01T00:00:00.000Z",
			expected: true,
		},
		{
			name: "workspace predates chat by days",
			workspace: "2026-03-10T10:00:00Z",
			chat: "2026-03-15T10:00:00Z",
			expected: false,
		},
	])("$name → $expected", ({ workspace, chat, expected }) => {
		expect(isWorkspaceAutoCreated(workspace, chat)).toBe(expected);
	});
});

describe("resolveArchiveAndDeleteAction", () => {
	it.each([
		{
			name: "auto-created workspace → proceed",
			workspaceCreatedAt: "2026-01-01T00:00:05Z",
			chatCreatedAt: "2026-01-01T00:00:00Z",
			expected: "proceed",
		},
		{
			name: "workspace predates chat → confirm",
			workspaceCreatedAt: "2025-12-01T00:00:00Z",
			chatCreatedAt: "2026-01-01T00:00:00Z",
			expected: "confirm",
		},
		{
			name: "chat not found in cache → confirm",
			workspaceCreatedAt: "2026-01-01T00:00:05Z",
			chatCreatedAt: undefined,
			expected: "confirm",
		},
	])("$name", async ({ workspaceCreatedAt, chatCreatedAt, expected }) => {
		const result = await resolveArchiveAndDeleteAction(
			async () => ({ created_at: workspaceCreatedAt }),
			() => chatCreatedAt,
		);
		expect(result).toBe(expected);
	});

	it("propagates workspace fetch errors", async () => {
		await expect(
			resolveArchiveAndDeleteAction(
				async () => {
					throw new Error("not found");
				},
				() => "2026-01-01T00:00:00Z",
			),
		).rejects.toThrow("not found");
	});
});
