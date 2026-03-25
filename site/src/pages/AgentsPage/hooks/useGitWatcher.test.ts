import { renderHook, waitFor } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceAgentStatus } from "#/api/typesGenerated";
import { useGitWatcher } from "./useGitWatcher";

vi.mock("api/api", () => ({
	watchChatGit: vi.fn(),
}));

import { watchChatGit } from "#/api/api";

const mockWatchChatGit = vi.mocked(watchChatGit);

class MockWebSocket {
	static readonly OPEN = 1;
	static readonly CLOSED = 3;

	readyState = MockWebSocket.OPEN;
	private listeners = new Map<string, Set<(...args: unknown[]) => void>>();

	addEventListener(event: string, handler: (...args: unknown[]) => void) {
		if (!this.listeners.has(event)) {
			this.listeners.set(event, new Set());
		}
		this.listeners.get(event)!.add(handler);
	}

	removeEventListener(event: string, handler: (...args: unknown[]) => void) {
		this.listeners.get(event)?.delete(handler);
	}

	send = vi.fn();
	close = vi.fn(() => {
		this.readyState = MockWebSocket.CLOSED;
	});

	simulateOpen() {
		this.readyState = MockWebSocket.OPEN;
		for (const handler of this.listeners.get("open") ?? []) {
			handler();
		}
	}

	simulateMessage(data: unknown) {
		for (const handler of this.listeners.get("message") ?? []) {
			handler({ data: JSON.stringify(data) });
		}
	}

	simulateClose() {
		this.readyState = MockWebSocket.CLOSED;
		for (const handler of this.listeners.get("close") ?? []) {
			handler();
		}
	}
}

function createMockSocket(): MockWebSocket {
	const socket = new MockWebSocket();
	mockWatchChatGit.mockReturnValue(socket as unknown as WebSocket);
	return socket;
}

describe("useGitWatcher", () => {
	beforeEach(() => {
		mockWatchChatGit.mockReset();
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("connects WebSocket when chatId is provided", () => {
		const socket = createMockSocket();

		const { result } = renderHook(() =>
			useGitWatcher({ chatId: "chat-123", agentStatus: "connected" }),
		);

		expect(mockWatchChatGit).toHaveBeenCalledWith("chat-123");
		expect(result.current.isConnected).toBe(false);

		act(() => socket.simulateOpen());
		expect(result.current.isConnected).toBe(true);
	});

	it("does not connect when chatId is undefined", () => {
		const { result } = renderHook(() =>
			useGitWatcher({ chatId: undefined, agentStatus: "connected" }),
		);

		expect(mockWatchChatGit).not.toHaveBeenCalled();
		expect(result.current.isConnected).toBe(false);
		expect(result.current.repositories.size).toBe(0);
	});

	it("does not connect when agentStatus is not connected", () => {
		createMockSocket();

		const { result } = renderHook(() =>
			useGitWatcher({ chatId: "chat-123", agentStatus: "disconnected" }),
		);

		expect(mockWatchChatGit).not.toHaveBeenCalled();
		expect(result.current.isConnected).toBe(false);
		expect(result.current.repositories.size).toBe(0);
	});

	it("does not connect when agentStatus is undefined", () => {
		createMockSocket();

		const { result } = renderHook(() =>
			useGitWatcher({ chatId: "chat-123", agentStatus: undefined }),
		);

		expect(mockWatchChatGit).not.toHaveBeenCalled();
		expect(result.current.isConnected).toBe(false);
		expect(result.current.repositories.size).toBe(0);
	});

	it("connects when agentStatus transitions to connected", () => {
		const socket = createMockSocket();

		const { result, rerender } = renderHook(
			({ agentStatus }: { agentStatus: WorkspaceAgentStatus | undefined }) =>
				useGitWatcher({ chatId: "chat-123", agentStatus }),
			{
				initialProps: {
					agentStatus: "connecting" as WorkspaceAgentStatus | undefined,
				},
			},
		);

		expect(mockWatchChatGit).not.toHaveBeenCalled();
		expect(result.current.isConnected).toBe(false);

		rerender({ agentStatus: "connected" });

		expect(mockWatchChatGit).toHaveBeenCalledWith("chat-123");

		act(() => socket.simulateOpen());
		expect(result.current.isConnected).toBe(true);
	});

	it("disconnects and stops reconnecting when agentStatus leaves connected", () => {
		vi.useFakeTimers();

		try {
			const socket = createMockSocket();

			const { result, rerender } = renderHook(
				({ agentStatus }: { agentStatus: WorkspaceAgentStatus | undefined }) =>
					useGitWatcher({ chatId: "chat-123", agentStatus }),
				{
					initialProps: {
						agentStatus: "connected" as WorkspaceAgentStatus | undefined,
					},
				},
			);

			act(() => socket.simulateOpen());
			expect(result.current.isConnected).toBe(true);

			// Transition agent away from connected.
			rerender({ agentStatus: "disconnected" });

			expect(socket.close).toHaveBeenCalled();
			expect(result.current.isConnected).toBe(false);

			// Simulate the browser firing the close event after
			// socket.close() — the disposedRef guard must prevent
			// the reconnect handler from scheduling a new attempt.
			mockWatchChatGit.mockClear();
			act(() => socket.simulateClose());
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatGit).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	it("populates repositories map from incoming changes messages", async () => {
		const socket = createMockSocket();

		const { result } = renderHook(() =>
			useGitWatcher({ chatId: "chat-123", agentStatus: "connected" }),
		);

		act(() => socket.simulateOpen());

		act(() => {
			socket.simulateMessage({
				type: "changes",
				repositories: [
					{
						repo_root: "/home/user/project-a",
						branch: "main",
						unified_diff: "diff content a",
					},
					{
						repo_root: "/home/user/project-b",
						branch: "feature",
						unified_diff: "diff content b",
					},
				],
			});
		});

		await waitFor(() => {
			expect(result.current.repositories.size).toBe(2);
		});

		const repoA = result.current.repositories.get("/home/user/project-a");
		expect(repoA).toEqual({
			repo_root: "/home/user/project-a",
			branch: "main",
			unified_diff: "diff content a",
		});

		const repoB = result.current.repositories.get("/home/user/project-b");
		expect(repoB).toEqual({
			repo_root: "/home/user/project-b",
			branch: "feature",
			unified_diff: "diff content b",
		});
	});

	it("evicts repos with removed: true", async () => {
		const socket = createMockSocket();

		const { result } = renderHook(() =>
			useGitWatcher({ chatId: "chat-123", agentStatus: "connected" }),
		);

		act(() => socket.simulateOpen());

		// First, populate with two repos.
		act(() => {
			socket.simulateMessage({
				type: "changes",
				repositories: [
					{
						repo_root: "/home/user/project-a",
						branch: "main",
					},
					{
						repo_root: "/home/user/project-b",
						branch: "feature",
					},
				],
			});
		});

		await waitFor(() => {
			expect(result.current.repositories.size).toBe(2);
		});

		// Remove one of them.
		act(() => {
			socket.simulateMessage({
				type: "changes",
				repositories: [
					{
						repo_root: "/home/user/project-a",
						branch: "",
						removed: true,
					},
				],
			});
		});

		await waitFor(() => {
			expect(result.current.repositories.size).toBe(1);
		});

		expect(result.current.repositories.has("/home/user/project-a")).toBe(false);
		expect(result.current.repositories.has("/home/user/project-b")).toBe(true);
	});

	it("reconnects with exponential backoff on close", () => {
		vi.useFakeTimers();

		try {
			const socket1 = createMockSocket();

			renderHook(() =>
				useGitWatcher({ chatId: "chat-123", agentStatus: "connected" }),
			);

			expect(mockWatchChatGit).toHaveBeenCalledTimes(1);
			act(() => socket1.simulateOpen());

			// Close the socket to trigger reconnection (attempt 0 → 1000ms).
			const socket2 = createMockSocket();
			act(() => socket1.simulateClose());

			// Before the timer fires, no reconnection yet.
			expect(mockWatchChatGit).toHaveBeenCalledTimes(1);
			act(() => vi.advanceTimersByTime(1000));
			expect(mockWatchChatGit).toHaveBeenCalledTimes(2);

			// Close again (attempt 1 → 2000ms).
			const socket3 = createMockSocket();
			act(() => socket2.simulateClose());

			act(() => vi.advanceTimersByTime(1999));
			expect(mockWatchChatGit).toHaveBeenCalledTimes(2);
			act(() => vi.advanceTimersByTime(1));
			expect(mockWatchChatGit).toHaveBeenCalledTimes(3);

			// Close again (attempt 2 → 4000ms).
			createMockSocket();
			act(() => socket3.simulateClose());

			act(() => vi.advanceTimersByTime(3999));
			expect(mockWatchChatGit).toHaveBeenCalledTimes(3);
			act(() => vi.advanceTimersByTime(1));
			expect(mockWatchChatGit).toHaveBeenCalledTimes(4);
		} finally {
			vi.useRealTimers();
		}
	});

	it("sends a refresh message over the socket", () => {
		const socket = createMockSocket();

		const { result } = renderHook(() =>
			useGitWatcher({ chatId: "chat-123", agentStatus: "connected" }),
		);

		act(() => socket.simulateOpen());

		act(() => result.current.refresh());

		expect(socket.send).toHaveBeenCalledTimes(1);
		expect(socket.send).toHaveBeenCalledWith(
			JSON.stringify({ type: "refresh" }),
		);
	});

	it("cleans up WebSocket and timers on unmount", () => {
		vi.useFakeTimers();

		try {
			const socket = createMockSocket();

			const { unmount } = renderHook(() =>
				useGitWatcher({ chatId: "chat-123", agentStatus: "connected" }),
			);

			act(() => socket.simulateOpen());
			expect(socket.close).not.toHaveBeenCalled();

			unmount();

			expect(socket.close).toHaveBeenCalledTimes(1);

			// Simulate the browser firing the close event after
			// socket.close() — the disposedRef guard must prevent
			// the reconnect handler from scheduling a new attempt.
			mockWatchChatGit.mockClear();
			act(() => socket.simulateClose());
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatGit).not.toHaveBeenCalled();
		} finally {
			vi.useRealTimers();
		}
	});

	it("resets repositories when chatId changes", async () => {
		const socket1 = createMockSocket();

		const { result, rerender } = renderHook(
			({ chatId }: { chatId: string | undefined }) =>
				useGitWatcher({ chatId, agentStatus: "connected" }),
			{ initialProps: { chatId: "chat-aaa" as string | undefined } },
		);

		act(() => socket1.simulateOpen());

		act(() => {
			socket1.simulateMessage({
				type: "changes",
				repositories: [
					{
						repo_root: "/home/user/project-a",
						branch: "main",
					},
				],
			});
		});

		await waitFor(() => {
			expect(result.current.repositories.size).toBe(1);
		});

		// The old socket should be closed when we switch chatId.
		const socket2 = createMockSocket();
		rerender({ chatId: "chat-bbb" });

		expect(socket1.close).toHaveBeenCalled();
		expect(mockWatchChatGit).toHaveBeenCalledWith("chat-bbb");

		// Repositories should be reset immediately after chatId changes.
		expect(result.current.repositories.size).toBe(0);

		// The new socket should work independently.
		act(() => socket2.simulateOpen());

		act(() => {
			socket2.simulateMessage({
				type: "changes",
				repositories: [
					{
						repo_root: "/home/user/project-x",
						branch: "develop",
					},
				],
			});
		});

		await waitFor(() => {
			expect(result.current.repositories.size).toBe(1);
		});
		expect(result.current.repositories.has("/home/user/project-x")).toBe(true);
	});

	it("stale close event after re-mount does not create duplicate connections", () => {
		vi.useFakeTimers();

		try {
			const socket1 = createMockSocket();

			const { result, rerender } = renderHook(
				({ chatId }: { chatId: string | undefined }) =>
					useGitWatcher({ chatId, agentStatus: "connected" }),
				{ initialProps: { chatId: "chat-aaa" as string | undefined } },
			);

			act(() => socket1.simulateOpen());
			expect(mockWatchChatGit).toHaveBeenCalledTimes(1);

			// Prepare socket2 for the re-mount triggered by chatId change.
			const socket2 = createMockSocket();
			rerender({ chatId: "chat-bbb" });

			expect(socket1.close).toHaveBeenCalled();
			expect(mockWatchChatGit).toHaveBeenCalledTimes(2);

			// Simulate socket1's close event arriving late (stale).
			// This must NOT clobber socketRef or schedule a reconnect.
			act(() => socket1.simulateClose());

			expect(mockWatchChatGit).toHaveBeenCalledTimes(2);

			// Advance timers to prove no reconnect was scheduled.
			act(() => vi.advanceTimersByTime(60_000));
			expect(mockWatchChatGit).toHaveBeenCalledTimes(2);

			// socket2 should still work: open sets isConnected,
			// messages update repositories.
			act(() => socket2.simulateOpen());
			expect(result.current.isConnected).toBe(true);

			act(() => {
				socket2.simulateMessage({
					type: "changes",
					repositories: [{ repo_root: "/repo", branch: "main" }],
				});
			});
			expect(result.current.repositories.size).toBe(1);
		} finally {
			vi.useRealTimers();
		}
	});

	it("preserves reference on duplicate messages", () => {
		const socket = createMockSocket();

		const { result } = renderHook(() =>
			useGitWatcher({ chatId: "chat-123", agentStatus: "connected" }),
		);

		act(() => socket.simulateOpen());

		const message = {
			type: "changes" as const,
			repositories: [
				{
					repo_root: "/repo",
					branch: "main",
					unified_diff: "diff1",
				},
			],
		};

		act(() => socket.simulateMessage(message));
		const ref1 = result.current.repositories;
		expect(ref1.size).toBe(1);

		// Sending the exact same data should not produce a new reference.
		act(() => socket.simulateMessage(message));
		expect(result.current.repositories).toBe(ref1);
	});

	it("single field change triggers update", () => {
		const socket = createMockSocket();

		const { result } = renderHook(() =>
			useGitWatcher({ chatId: "chat-123", agentStatus: "connected" }),
		);

		act(() => socket.simulateOpen());

		const base = {
			repo_root: "/repo",
			branch: "main",
			remote_origin: "git@github.com:org/repo.git",
			unified_diff: "diff1",
		};

		act(() => {
			socket.simulateMessage({
				type: "changes",
				repositories: [base],
			});
		});
		let ref = result.current.repositories;
		expect(ref.get("/repo")?.branch).toBe("main");

		// Changing only branch.
		act(() => {
			socket.simulateMessage({
				type: "changes",
				repositories: [{ ...base, branch: "feature" }],
			});
		});
		expect(result.current.repositories).not.toBe(ref);
		expect(result.current.repositories.get("/repo")?.branch).toBe("feature");
		ref = result.current.repositories;

		// Changing only remote_origin.
		act(() => {
			socket.simulateMessage({
				type: "changes",
				repositories: [
					{
						...base,
						branch: "feature",
						remote_origin: "https://github.com/org/repo",
					},
				],
			});
		});
		expect(result.current.repositories).not.toBe(ref);
		ref = result.current.repositories;

		// Changing only unified_diff.
		act(() => {
			socket.simulateMessage({
				type: "changes",
				repositories: [
					{
						...base,
						branch: "feature",
						remote_origin: "https://github.com/org/repo",
						unified_diff: "diff2",
					},
				],
			});
		});
		expect(result.current.repositories).not.toBe(ref);
		expect(result.current.repositories.get("/repo")?.unified_diff).toBe(
			"diff2",
		);
	});

	it("removing unknown repo preserves reference", () => {
		const socket = createMockSocket();

		const { result } = renderHook(() =>
			useGitWatcher({ chatId: "chat-123", agentStatus: "connected" }),
		);

		act(() => socket.simulateOpen());

		act(() => {
			socket.simulateMessage({
				type: "changes",
				repositories: [
					{
						repo_root: "/repo",
						branch: "main",
						unified_diff: "diff1",
					},
				],
			});
		});
		const ref1 = result.current.repositories;
		expect(ref1.size).toBe(1);

		// Removing a repo_root that was never added should be a no-op.
		act(() => {
			socket.simulateMessage({
				type: "changes",
				repositories: [
					{
						repo_root: "/unknown",
						branch: "",
						removed: true,
					},
				],
			});
		});
		expect(result.current.repositories).toBe(ref1);
	});
});
