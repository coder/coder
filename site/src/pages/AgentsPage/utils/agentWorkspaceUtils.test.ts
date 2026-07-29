import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("sonner", () => ({
	toast: {
		error: vi.fn(),
		info: vi.fn(),
		success: vi.fn(),
		warning: vi.fn(),
	},
}));

import { toast } from "sonner";
import {
	PrebuildsSystemUserID,
	type Workspace,
	type WorkspaceBuild,
} from "#/api/typesGenerated";
import {
	ArchiveAndDeleteError,
	archiveChatAndDeleteWorkspace,
	isWorkspaceAutoCreated,
	isWorkspaceNotFound,
	notifyArchiveAndDeleteFailed,
	notifyDeleteQueueState,
	resolveArchiveAndDeleteAction,
	shouldNavigateAfterArchive,
	workspaceAcquiredAt,
} from "./agentWorkspaceUtils";

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
				initiator_id: PrebuildsSystemUserID,
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
				initiator_id: PrebuildsSystemUserID,
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
				initiator_id: PrebuildsSystemUserID,
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
					initiator_id: PrebuildsSystemUserID,
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
					initiator_id: PrebuildsSystemUserID,
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
					initiator_id: PrebuildsSystemUserID,
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
	const BUILD_OK = {
		job: { queue_position: 0, queue_size: 1 },
	} as unknown as WorkspaceBuild;

	it("archives and deletes when both succeed, deleting first", async () => {
		const callOrder: string[] = [];
		const doArchive = vi.fn(async () => {
			callOrder.push("archive");
		});
		const doDelete = vi.fn(async () => {
			callOrder.push("delete");
			return BUILD_OK;
		});

		await expect(
			archiveChatAndDeleteWorkspace(
				"chat-1",
				"workspace-1",
				doArchive,
				doDelete,
			),
		).resolves.toEqual({
			chatId: "chat-1",
			workspaceId: "workspace-1",
			deleteBuild: BUILD_OK,
		});
		expect(doArchive).toHaveBeenCalledTimes(1);
		expect(doArchive).toHaveBeenCalledWith("chat-1");
		expect(doDelete).toHaveBeenCalledTimes(1);
		expect(doDelete).toHaveBeenCalledWith("workspace-1");
		expect(callOrder).toEqual(["delete", "archive"]);
	});

	it("archives even when delete returns 404, with null deleteBuild", async () => {
		const callOrder: string[] = [];
		const doArchive = vi.fn(async () => {
			callOrder.push("archive");
		});
		const doDelete = vi.fn(async () => {
			callOrder.push("delete");
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
		).resolves.toEqual({
			chatId: "chat-1",
			workspaceId: "workspace-1",
			deleteBuild: null,
		});
		expect(callOrder).toEqual(["delete", "archive"]);
	});

	it("archives even when delete returns 410, with null deleteBuild", async () => {
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
		).resolves.toEqual({
			chatId: "chat-1",
			workspaceId: "workspace-1",
			deleteBuild: null,
		});
		expect(doArchive).toHaveBeenCalledTimes(1);
		expect(doDelete).toHaveBeenCalledTimes(1);
	});

	it("wraps non-404-or-410 delete failures and skips archive", async () => {
		const doArchive = vi.fn(async () => undefined);
		const cause = {
			isAxiosError: true,
			response: {
				status: 500,
				data: { message: "Internal server error" },
			},
		};
		const doDelete = vi.fn(async () => {
			throw cause;
		});

		const promise = archiveChatAndDeleteWorkspace(
			"chat-1",
			"workspace-1",
			doArchive,
			doDelete,
		);
		await expect(promise).rejects.toBeInstanceOf(ArchiveAndDeleteError);
		const err = await promise.catch((e: unknown) => e);
		expect((err as ArchiveAndDeleteError).step).toBe("delete");
		expect((err as ArchiveAndDeleteError).cause).toBe(cause);
		expect(doDelete).toHaveBeenCalledTimes(1);
		expect(doArchive).not.toHaveBeenCalled();
	});

	it("wraps archive failures that follow a successful delete", async () => {
		const cause = new Error("archive failed");
		const doArchive = vi.fn(async () => {
			throw cause;
		});
		const doDelete = vi.fn(async () => BUILD_OK);

		const promise = archiveChatAndDeleteWorkspace(
			"chat-1",
			"workspace-1",
			doArchive,
			doDelete,
		);
		await expect(promise).rejects.toBeInstanceOf(ArchiveAndDeleteError);
		const err = await promise.catch((e: unknown) => e);
		expect((err as ArchiveAndDeleteError).step).toBe("archive");
		expect((err as ArchiveAndDeleteError).cause).toBe(cause);
		expect((err as ArchiveAndDeleteError).deleteEnqueued).toBe(true);
		expect(doDelete).toHaveBeenCalledTimes(1);
		expect(doArchive).toHaveBeenCalledTimes(1);
	});

	it("marks archive failures with deleteEnqueued=false when delete was skipped", async () => {
		const doArchive = vi.fn(async () => {
			throw new Error("archive failed");
		});
		const doDelete = vi.fn(async () => {
			throw {
				isAxiosError: true,
				response: { status: 410, data: { message: "gone" } },
			};
		});

		const promise = archiveChatAndDeleteWorkspace(
			"chat-1",
			"workspace-1",
			doArchive,
			doDelete,
		);
		const err = (await promise.catch(
			(e: unknown) => e,
		)) as ArchiveAndDeleteError;
		expect(err.step).toBe("archive");
		expect(err.deleteEnqueued).toBe(false);
	});

	it("returns the delete build payload on success", async () => {
		const build = {
			job: { queue_position: 4, queue_size: 7 },
		} as unknown as WorkspaceBuild;
		const doArchive = vi.fn(async () => undefined);
		const doDelete = vi.fn(async () => build);

		const result = await archiveChatAndDeleteWorkspace(
			"chat-1",
			"workspace-1",
			doArchive,
			doDelete,
		);
		expect(result.deleteBuild).toBe(build);
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
					initiator_id: PrebuildsSystemUserID,
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
					initiator_id: PrebuildsSystemUserID,
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

const makeWorkspace = (overrides: Partial<Workspace> = {}): Workspace =>
	({
		id: "ws-1",
		name: "my-workspace",
		owner_name: "alice",
		...overrides,
	}) as Workspace;

const makeDeleteBuild = (matched?: {
	count: number;
	available?: number;
}): WorkspaceBuild =>
	({
		job: { queue_position: 0, queue_size: 0 },
		matched_provisioners: matched
			? { count: matched.count, available: matched.available ?? matched.count }
			: undefined,
	}) as unknown as WorkspaceBuild;

describe("notifyDeleteQueueState", () => {
	const toastWarning = toast.warning as unknown as ReturnType<typeof vi.fn>;
	const toastInfo = toast.info as unknown as ReturnType<typeof vi.fn>;
	const toastSuccess = toast.success as unknown as ReturnType<typeof vi.fn>;

	beforeEach(() => {
		toastWarning.mockClear();
		toastInfo.mockClear();
		toastSuccess.mockClear();
	});

	it("is silent when no build is returned (workspace already gone)", () => {
		notifyDeleteQueueState(makeWorkspace(), null);
		expect(toastWarning).not.toHaveBeenCalled();
		expect(toastInfo).not.toHaveBeenCalled();
		expect(toastSuccess).not.toHaveBeenCalled();
	});

	it("is silent when workspace is not in cache", () => {
		notifyDeleteQueueState(undefined, makeDeleteBuild({ count: 0 }));
		expect(toastWarning).not.toHaveBeenCalled();
	});

	it("is silent when matched_provisioners is absent (older servers)", () => {
		notifyDeleteQueueState(makeWorkspace(), makeDeleteBuild());
		expect(toastWarning).not.toHaveBeenCalled();
	});

	it("is silent on the happy path (at least one matching provisioner)", () => {
		notifyDeleteQueueState(makeWorkspace(), makeDeleteBuild({ count: 2 }));
		expect(toastWarning).not.toHaveBeenCalled();
	});

	it("warns when no matching provisioners exist (count = 0)", () => {
		notifyDeleteQueueState(
			makeWorkspace({ name: "stuck-ws" }),
			makeDeleteBuild({ count: 0 }),
		);
		expect(toastWarning).toHaveBeenCalledTimes(1);
		const message = toastWarning.mock.calls[0][0] as string;
		expect(message).toContain("stuck-ws");
		expect(message).toContain("no matching provisioners");
	});
});

describe("notifyArchiveAndDeleteFailed", () => {
	const toastError = toast.error as unknown as ReturnType<typeof vi.fn>;

	beforeEach(() => {
		toastError.mockClear();
	});

	it("shows a generic delete-failed toast when workspace is not in cache", () => {
		const onOpen = vi.fn();
		notifyArchiveAndDeleteFailed(
			undefined,
			new ArchiveAndDeleteError("delete", {}),
			onOpen,
		);
		expect(toastError).toHaveBeenCalledTimes(1);
		expect(toastError.mock.calls[0][0]).toContain(
			"Failed to delete workspace.",
		);
		expect(toastError.mock.calls[0][1]).toBeUndefined();
	});

	it("includes workspace name and an Open workspace action when delete fails", () => {
		const onOpen = vi.fn();
		notifyArchiveAndDeleteFailed(
			makeWorkspace({ name: "left-behind", owner_name: "bob" }),
			new ArchiveAndDeleteError("delete", {}),
			onOpen,
		);
		expect(toastError).toHaveBeenCalledTimes(1);
		const [message, options] = toastError.mock.calls[0] as [
			string,
			{
				description: string;
				action: { label: string; onClick: () => void };
			},
		];
		expect(message).toContain("left-behind");
		expect(options.description).toContain("not archived");
		expect(options.action.label).toBe("Open workspace");
		options.action.onClick();
		expect(onOpen).toHaveBeenCalledWith("/@bob/left-behind");
	});

	it("announces partial success when only the archive step fails after enqueue", () => {
		const onOpen = vi.fn();
		notifyArchiveAndDeleteFailed(
			makeWorkspace({ name: "deleting-ws" }),
			new ArchiveAndDeleteError("archive", new Error("forbidden"), true),
			onOpen,
		);
		expect(toastError).toHaveBeenCalledTimes(1);
		const [message, options] = toastError.mock.calls[0] as [string, undefined];
		expect(message).toContain("deleting-ws");
		expect(message).toContain("Deleting");
		expect(message).toContain("failed to archive");
		expect(options).toBeUndefined();
		expect(onOpen).not.toHaveBeenCalled();
	});

	it("omits the 'Deleting' claim when the workspace was already gone (delete swallowed)", () => {
		const onOpen = vi.fn();
		notifyArchiveAndDeleteFailed(
			makeWorkspace({ name: "already-gone" }),
			new ArchiveAndDeleteError("archive", new Error("forbidden"), false),
			onOpen,
		);
		expect(toastError).toHaveBeenCalledTimes(1);
		const message = toastError.mock.calls[0][0] as string;
		expect(message).toContain("already-gone");
		expect(message).toContain("Failed to archive");
		expect(message).not.toContain("Deleting");
	});

	it("handles archive-step failure with no workspace in cache", () => {
		const onOpen = vi.fn();
		notifyArchiveAndDeleteFailed(
			undefined,
			new ArchiveAndDeleteError("archive", new Error("forbidden"), true),
			onOpen,
		);
		expect(toastError).toHaveBeenCalledTimes(1);
		const [message, options] = toastError.mock.calls[0] as [string, undefined];
		expect(message).toContain("the workspace");
		expect(message).toContain("failed to archive");
		expect(options).toBeUndefined();
	});

	it("surfaces the original error's message when present", () => {
		const onOpen = vi.fn();
		notifyArchiveAndDeleteFailed(
			makeWorkspace(),
			new ArchiveAndDeleteError(
				"delete",
				new Error("template version archived"),
			),
			onOpen,
		);
		const message = toastError.mock.calls[0][0] as string;
		expect(message).toContain("template version archived");
	});

	it("falls back to the delete branch (with action) for non-tagged errors", () => {
		const onOpen = vi.fn();
		notifyArchiveAndDeleteFailed(
			makeWorkspace({ name: "raw-ws", owner_name: "carol" }),
			new Error("raw"),
			onOpen,
		);
		expect(toastError).toHaveBeenCalledTimes(1);
		const [message, options] = toastError.mock.calls[0] as [
			string,
			{
				description: string;
				action: { label: string; onClick: () => void };
			},
		];
		expect(message).toContain("raw");
		expect(options.action.label).toBe("Open workspace");
		options.action.onClick();
		expect(onOpen).toHaveBeenCalledWith("/@carol/raw-ws");
	});
});
