import { describe, expect, it, vi } from "vitest";
import {
	archiveChatAndDeleteWorkspace,
	isWorkspaceAutoCreated,
	isWorkspaceNotFound,
	resolveArchiveAndDeleteAction,
	shouldNavigateAfterArchive,
	workspaceAcquiredAt,
} from "./agentWorkspaceUtils";

const PREBUILDS_USER = "c42fdf75-3097-471c-8c33-fb52454d81c0";
const REAL_USER = "11111111-2222-3333-4444-555555555555";

describe("workspaceAcquiredAt", () => {
	it("returns workspace.created_at when no builds exist", () => {
		const ws = { created_at: "2026-01-01T00:00:00Z" };
		expect(workspaceAcquiredAt(ws, [])).toBe("2026-01-01T00:00:00Z");
	});

	it("returns workspace.created_at when build #1 was initiated by a real user", () => {
		const ws = { created_at: "2026-01-01T00:00:00Z" };
		const builds = [
			{
				build_number: 1,
				initiator_id: REAL_USER,
				created_at: "2026-01-01T00:00:01Z",
			},
		];
		expect(workspaceAcquiredAt(ws, builds)).toBe("2026-01-01T00:00:00Z");
	});

	it("returns build #2 created_at when build #1 was initiated by the prebuilds user", () => {
		// Workspace.created_at predates the chat (the prebuild was
		// provisioned long before the chat existed), but build #2 is
		// the claim and that's the moment the chat acquired the
		// workspace.
		const ws = { created_at: "2026-01-01T08:00:00Z" };
		const builds = [
			{
				build_number: 2,
				initiator_id: REAL_USER,
				created_at: "2026-01-01T12:00:05Z",
			},
			{
				build_number: 1,
				initiator_id: PREBUILDS_USER,
				created_at: "2026-01-01T08:00:01Z",
			},
		];
		expect(workspaceAcquiredAt(ws, builds)).toBe("2026-01-01T12:00:05Z");
	});

	it("returns null when prebuild has no claim build yet", () => {
		const ws = { created_at: "2026-01-01T08:00:00Z" };
		const builds = [
			{
				build_number: 1,
				initiator_id: PREBUILDS_USER,
				created_at: "2026-01-01T08:00:01Z",
			},
		];
		expect(workspaceAcquiredAt(ws, builds)).toBeNull();
	});

	it("ignores extra builds beyond #1 and #2", () => {
		const ws = { created_at: "2026-01-01T08:00:00Z" };
		const builds = [
			{
				build_number: 5,
				initiator_id: REAL_USER,
				created_at: "2026-02-01T00:00:00Z",
			},
			{
				build_number: 4,
				initiator_id: REAL_USER,
				created_at: "2026-01-15T00:00:00Z",
			},
			{
				build_number: 3,
				initiator_id: REAL_USER,
				created_at: "2026-01-10T00:00:00Z",
			},
			{
				build_number: 2,
				initiator_id: REAL_USER,
				created_at: "2026-01-01T12:00:05Z",
			},
			{
				build_number: 1,
				initiator_id: PREBUILDS_USER,
				created_at: "2026-01-01T08:00:01Z",
			},
		];
		expect(workspaceAcquiredAt(ws, builds)).toBe("2026-01-01T12:00:05Z");
	});
});

describe("isWorkspaceAutoCreated", () => {
	it.each([
		{
			name: "from-scratch workspace created after chat",
			workspace: { created_at: "2026-01-01T00:00:05Z" },
			builds: [
				{
					build_number: 1,
					initiator_id: REAL_USER,
					created_at: "2026-01-01T00:00:05Z",
				},
			],
			chat: "2026-01-01T00:00:00Z",
			expected: true,
		},
		{
			name: "from-scratch workspace created at same time as chat",
			workspace: { created_at: "2026-01-01T12:00:00Z" },
			builds: [
				{
					build_number: 1,
					initiator_id: REAL_USER,
					created_at: "2026-01-01T12:00:00Z",
				},
			],
			chat: "2026-01-01T12:00:00Z",
			expected: true,
		},
		{
			name: "from-scratch workspace created before chat",
			workspace: { created_at: "2026-01-01T11:59:59Z" },
			builds: [
				{
					build_number: 1,
					initiator_id: REAL_USER,
					created_at: "2026-01-01T11:59:59Z",
				},
			],
			chat: "2026-01-01T12:00:00Z",
			expected: false,
		},
		// Prebuild claim cases: workspace.created_at predates the
		// chat, but build #2 (the claim) happened after the chat.
		{
			name: "prebuild claimed after chat",
			workspace: { created_at: "2026-01-01T08:00:00Z" },
			builds: [
				{
					build_number: 2,
					initiator_id: REAL_USER,
					created_at: "2026-01-01T12:00:05Z",
				},
				{
					build_number: 1,
					initiator_id: PREBUILDS_USER,
					created_at: "2026-01-01T08:00:01Z",
				},
			],
			chat: "2026-01-01T12:00:00Z",
			expected: true,
		},
		{
			name: "prebuild claimed before chat",
			workspace: { created_at: "2026-01-01T08:00:00Z" },
			builds: [
				{
					build_number: 2,
					initiator_id: REAL_USER,
					created_at: "2026-01-01T11:00:00Z",
				},
				{
					build_number: 1,
					initiator_id: PREBUILDS_USER,
					created_at: "2026-01-01T08:00:01Z",
				},
			],
			chat: "2026-01-01T12:00:00Z",
			expected: false,
		},
		{
			name: "unclaimed prebuild treated as not auto-created",
			workspace: { created_at: "2026-01-01T08:00:00Z" },
			builds: [
				{
					build_number: 1,
					initiator_id: PREBUILDS_USER,
					created_at: "2026-01-01T08:00:01Z",
				},
			],
			chat: "2026-01-01T12:00:00Z",
			expected: false,
		},
		{
			// Defensive: build history empty. Fall back to
			// workspace.created_at so we still allow the proceed
			// path in the common case rather than blocking on data.
			name: "no builds, falls back to workspace.created_at",
			workspace: { created_at: "2026-01-01T12:00:05Z" },
			builds: [],
			chat: "2026-01-01T12:00:00Z",
			expected: true,
		},
	])("$name → $expected", ({ workspace, builds, chat, expected }) => {
		expect(isWorkspaceAutoCreated(workspace, builds, chat)).toBe(expected);
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
			name: "from-scratch workspace created after chat → proceed",
			workspace: { created_at: "2026-01-01T00:00:05Z" },
			builds: [
				{
					build_number: 1,
					initiator_id: REAL_USER,
					created_at: "2026-01-01T00:00:05Z",
				},
			],
			chatCreatedAt: "2026-01-01T00:00:00Z",
			expected: "proceed",
		},
		{
			name: "workspace predates chat → confirm",
			workspace: { created_at: "2025-12-01T00:00:00Z" },
			builds: [
				{
					build_number: 1,
					initiator_id: REAL_USER,
					created_at: "2025-12-01T00:00:00Z",
				},
			],
			chatCreatedAt: "2026-01-01T00:00:00Z",
			expected: "confirm",
		},
		{
			name: "chat not found in cache → confirm",
			workspace: { created_at: "2026-01-01T00:00:05Z" },
			builds: [
				{
					build_number: 1,
					initiator_id: REAL_USER,
					created_at: "2026-01-01T00:00:05Z",
				},
			],
			chatCreatedAt: undefined,
			expected: "confirm",
		},
		{
			// The bug this PR fixes: workspace.created_at predates
			// the chat (the prebuild was provisioned earlier) but
			// build #2 is the claim and happened after the chat.
			name: "prebuild claimed after chat → proceed",
			workspace: { created_at: "2025-12-15T00:00:00Z" },
			builds: [
				{
					build_number: 2,
					initiator_id: REAL_USER,
					created_at: "2026-01-01T00:00:05Z",
				},
				{
					build_number: 1,
					initiator_id: PREBUILDS_USER,
					created_at: "2025-12-15T00:00:00Z",
				},
			],
			chatCreatedAt: "2026-01-01T00:00:00Z",
			expected: "proceed",
		},
		{
			name: "prebuild claimed before chat → confirm",
			workspace: { created_at: "2025-12-15T00:00:00Z" },
			builds: [
				{
					build_number: 2,
					initiator_id: REAL_USER,
					created_at: "2025-12-31T00:00:00Z",
				},
				{
					build_number: 1,
					initiator_id: PREBUILDS_USER,
					created_at: "2025-12-15T00:00:00Z",
				},
			],
			chatCreatedAt: "2026-01-01T00:00:00Z",
			expected: "confirm",
		},
	])("$name", async ({ workspace, builds, chatCreatedAt, expected }) => {
		const result = await resolveArchiveAndDeleteAction(
			async () => workspace,
			async () => builds,
			() => chatCreatedAt,
		);
		expect(result).toBe(expected);
	});

	it("does not fetch builds when the chat is not in the cache", async () => {
		const fetchBuilds = vi.fn(async () => []);
		const result = await resolveArchiveAndDeleteAction(
			async () => ({ created_at: "2026-01-01T00:00:00Z" }),
			fetchBuilds,
			() => undefined,
		);
		expect(result).toBe("confirm");
		expect(fetchBuilds).not.toHaveBeenCalled();
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
				async () => [],
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
				async () => [],
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
				async () => [],
				() => "2026-01-01T00:00:00Z",
			),
		).resolves.toBe("archive-only");
	});

	it("returns archive-only when the builds fetch returns 404", async () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 404,
				data: { message: "Workspace not found" },
			},
		};

		await expect(
			resolveArchiveAndDeleteAction(
				async () => ({ created_at: "2026-01-01T00:00:00Z" }),
				async () => {
					throw error;
				},
				() => "2026-01-01T00:00:00Z",
			),
		).resolves.toBe("archive-only");
	});

	it("propagates non-404-or-410 builds fetch errors", async () => {
		const error = {
			isAxiosError: true,
			response: {
				status: 500,
				data: { message: "Internal server error" },
			},
		};

		await expect(
			resolveArchiveAndDeleteAction(
				async () => ({ created_at: "2026-01-01T00:00:00Z" }),
				async () => {
					throw error;
				},
				() => "2026-01-01T00:00:00Z",
			),
		).rejects.toBe(error);
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
	])("$name → $expected", ({
		activeChatId,
		archivedChatId,
		activeRootChatId,
		expected,
	}) => {
		expect(
			shouldNavigateAfterArchive(
				activeChatId,
				archivedChatId,
				activeRootChatId,
			),
		).toBe(expected);
	});
});
