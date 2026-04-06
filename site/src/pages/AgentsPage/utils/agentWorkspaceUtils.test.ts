import { describe, expect, it, vi } from "vitest";
import {
	archiveChatAndDeleteWorkspace,
	isWorkspaceAutoCreated,
	isWorkspaceNotFound,
	resolveArchiveAndDeleteAction,
	shouldNavigateAfterArchive,
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

describe("isWorkspaceNotFound", () => {
	it("returns true for Axios-style 404 Not Found errors", () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 404,
				data: { message: "Workspace not found" },
			},
		};

		expect(isWorkspaceNotFound(error)).toBe(true);
	});

	it("returns true for Axios-style 410 errors", () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 410,
				data: { message: "Workspace gone" },
			},
		};

		expect(isWorkspaceNotFound(error)).toBe(true);
	});

	it("returns false for Axios-style non-404-or-410 errors", () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 500,
				data: { message: "Internal server error" },
			},
		};

		expect(isWorkspaceNotFound(error)).toBe(false);
	});

	it("returns false for axios errors without a response (network error)", () => {
		const error = {
			isAxiosError: true,
			response: undefined,
		};

		expect(isWorkspaceNotFound(error)).toBe(false);
	});

	it("returns false for plain Error objects", () => {
		expect(isWorkspaceNotFound(new Error("Workspace not found"))).toBe(false);
	});

	it("returns false for non-error values", () => {
		expect(isWorkspaceNotFound("nope")).toBe(false);
		expect(isWorkspaceNotFound(null)).toBe(false);
	});
});

describe("archiveChatAndDeleteWorkspace", () => {
	it("archives and deletes when both succeed", async () => {
		const callOrder: string[] = [];
		const doArchive = vi.fn(async () => {
			callOrder.push("archive");
		});
		const doDelete = vi.fn(async () => {
			callOrder.push("delete");
		});

		await expect(
			archiveChatAndDeleteWorkspace(
				"chat-1",
				"workspace-1",
				doArchive,
				doDelete,
			),
		).resolves.toEqual({ chatId: "chat-1", workspaceId: "workspace-1" });
		expect(doArchive).toHaveBeenCalledTimes(1);
		expect(doArchive).toHaveBeenCalledWith("chat-1");
		expect(doDelete).toHaveBeenCalledTimes(1);
		expect(doDelete).toHaveBeenCalledWith("workspace-1");
		expect(callOrder).toEqual(["archive", "delete"]);
	});

	it("succeeds when delete returns 404", async () => {
		const doArchive = vi.fn(async () => undefined);
		const doDelete = vi.fn(async () => {
			throw {
				isAxiosError: true,
				response: {
					status: 404,
					data: { message: "Workspace not found" },
				},
			};
		});

		await expect(
			archiveChatAndDeleteWorkspace(
				"chat-1",
				"workspace-1",
				doArchive,
				doDelete,
			),
		).resolves.toEqual({ chatId: "chat-1", workspaceId: "workspace-1" });
		expect(doArchive).toHaveBeenCalledTimes(1);
		expect(doDelete).toHaveBeenCalledTimes(1);
	});

	it("succeeds when delete returns 410", async () => {
		const doArchive = vi.fn(async () => undefined);
		const doDelete = vi.fn(async () => {
			throw {
				isAxiosError: true,
				response: {
					status: 410,
					data: { message: "Workspace gone" },
				},
			};
		});

		await expect(
			archiveChatAndDeleteWorkspace(
				"chat-1",
				"workspace-1",
				doArchive,
				doDelete,
			),
		).resolves.toEqual({ chatId: "chat-1", workspaceId: "workspace-1" });
		expect(doArchive).toHaveBeenCalledTimes(1);
		expect(doDelete).toHaveBeenCalledTimes(1);
	});

	it("throws when delete returns non-404-or-410 error", async () => {
		const doArchive = vi.fn(async () => undefined);
		const error = {
			isAxiosError: true,
			response: {
				status: 500,
				data: { message: "Internal server error" },
			},
		};
		const doDelete = vi.fn(async () => {
			throw error;
		});

		await expect(
			archiveChatAndDeleteWorkspace(
				"chat-1",
				"workspace-1",
				doArchive,
				doDelete,
			),
		).rejects.toBe(error);
		expect(doArchive).toHaveBeenCalledTimes(1);
		expect(doDelete).toHaveBeenCalledTimes(1);
	});

	it("throws when archive fails without attempting delete", async () => {
		const error = new Error("archive failed");
		const doArchive = vi.fn(async () => {
			throw error;
		});
		const doDelete = vi.fn(async () => undefined);

		await expect(
			archiveChatAndDeleteWorkspace(
				"chat-1",
				"workspace-1",
				doArchive,
				doDelete,
			),
		).rejects.toBe(error);
		expect(doArchive).toHaveBeenCalledTimes(1);
		expect(doDelete).not.toHaveBeenCalled();
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

	it("propagates non-404-or-410 workspace fetch errors", async () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 500,
				data: { message: "Internal server error" },
			},
		};

		await expect(
			resolveArchiveAndDeleteAction(
				async () => {
					throw error;
				},
				() => "2026-01-01T00:00:00Z",
			),
		).rejects.toBe(error);
	});

	it("returns archive-only when the workspace fetch returns 404", async () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 404,
				data: { message: "Workspace not found" },
			},
		};

		await expect(
			resolveArchiveAndDeleteAction(
				async () => {
					throw error;
				},
				() => "2026-01-01T00:00:00Z",
			),
		).resolves.toBe("archive-only");
	});

	it("returns archive-only when the workspace fetch returns 410", async () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 410,
				data: { message: "Workspace gone" },
			},
		};

		await expect(
			resolveArchiveAndDeleteAction(
				async () => {
					throw error;
				},
				() => "2026-01-01T00:00:00Z",
			),
		).resolves.toBe("archive-only");
	});
});

describe("shouldNavigateAfterArchive", () => {
	it.each([
		{
			name: "user still viewing archived chat",
			activeChatId: "abc-123",
			archivedChatId: "abc-123",
			expected: true,
		},
		{
			name: "user navigated to a different chat",
			activeChatId: "xyz-456",
			archivedChatId: "abc-123",
			expected: false,
		},
		{
			name: "user navigated to /agents root (no active chat)",
			activeChatId: undefined,
			archivedChatId: "abc-123",
			expected: false,
		},
		{
			name: "user viewing sub-agent of archived parent",
			activeChatId: "sub-agent-1",
			archivedChatId: "parent-abc",
			activeRootChatId: "parent-abc",
			expected: true,
		},
		{
			name: "user viewing sub-agent of a different parent",
			activeChatId: "sub-agent-1",
			archivedChatId: "parent-abc",
			activeRootChatId: "parent-other",
			expected: false,
		},
		{
			name: "root chat ID not available (cache cleared)",
			activeChatId: "sub-agent-1",
			archivedChatId: "parent-abc",
			activeRootChatId: undefined,
			expected: false,
		},
	])(
		"$name → $expected",
		({ activeChatId, archivedChatId, activeRootChatId, expected }) => {
			expect(
				shouldNavigateAfterArchive(
					activeChatId,
					archivedChatId,
					activeRootChatId,
				),
			).toBe(expected);
		},
	);
});
