import { useEffect, useRef, useState } from "react";
import { watchChatGit } from "#/api/api";
import type {
	WorkspaceAgentGitClientMessage,
	WorkspaceAgentGitServerMessage,
	WorkspaceAgentRepoChanges,
	WorkspaceAgentStatus,
} from "#/api/typesGenerated";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";

// Compile-time guard: ensures the bailout comparison in setRepositories
// covers every data field. If WorkspaceAgentRepoChanges gains a new
// field, this will error until the comparison is updated.
type _ComparedRepoFields = Omit<
	WorkspaceAgentRepoChanges,
	"repo_root" | "removed"
>;
const _repoFieldGuard: Record<keyof _ComparedRepoFields, true> = {
	branch: true,
	remote_origin: true,
	unified_diff: true,
};

interface UseGitWatcherOptions {
	chatId: string | undefined;
	agentStatus: WorkspaceAgentStatus | undefined;
}

interface UseGitWatcherResult {
	/** Current repo state, keyed by repo root path. */
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	/** Whether the WebSocket is currently connected. */
	isConnected: boolean;
	/** Send a refresh request. Returns true if sent, false if disconnected. */
	refresh: () => boolean;
}

export function useGitWatcher({
	chatId,
	agentStatus,
}: UseGitWatcherOptions): UseGitWatcherResult {
	const [repositories, setRepositories] = useState<
		ReadonlyMap<string, WorkspaceAgentRepoChanges>
	>(new Map());
	const [isConnected, setIsConnected] = useState(false);

	const socketRef = useRef<WebSocket | null>(null);

	const sendMessage = (msg: WorkspaceAgentGitClientMessage): boolean => {
		const socket = socketRef.current;
		if (socket && socket.readyState === WebSocket.OPEN) {
			socket.send(JSON.stringify(msg));
			return true;
		}
		return false;
	};

	const refresh = (): boolean => {
		return sendMessage({ type: "refresh" });
	};

	useEffect(() => {
		if (!chatId || agentStatus !== "connected") {
			return;
		}

		const activeChatId = chatId;

		const dispose = createReconnectingWebSocket({
			connect() {
				const socket = watchChatGit(activeChatId);
				socketRef.current = socket;

				socket.addEventListener("message", (event) => {
					// Ignore messages from superseded connections.
					if (socketRef.current !== socket) {
						return;
					}
					let data: WorkspaceAgentGitServerMessage;
					try {
						data = JSON.parse(
							String((event as MessageEvent).data),
						) as WorkspaceAgentGitServerMessage;
					} catch {
						// Ignore unparsable messages.
						return;
					}

					if (data.type === "changes" && data.repositories) {
						setRepositories((prev) => {
							let changed = false;
							const next = new Map(prev);
							for (const repo of data.repositories!) {
								if (repo.removed) {
									if (next.has(repo.repo_root)) {
										next.delete(repo.repo_root);
										changed = true;
									}
								} else {
									const existing = next.get(repo.repo_root);
									if (
										!existing ||
										existing.branch !== repo.branch ||
										existing.remote_origin !== repo.remote_origin ||
										existing.unified_diff !== repo.unified_diff
									) {
										next.set(repo.repo_root, repo);
										changed = true;
									}
								}
							}
							return changed ? next : prev;
						});
					} else if (data.type === "error") {
						console.warn("[useGitWatcher] server error:", data.message);
					}
				});

				return socket;
			},

			onOpen() {
				setIsConnected(true);
			},

			onDisconnect() {
				setIsConnected(false);
				socketRef.current = null;
			},

			// 30s cap instead of the utility default 10s. The git
			// endpoint may be slow to respond after a workspace wakes.
			maxMs: 30_000,
		});

		return () => {
			// dispose() suppresses onDisconnect, so reset state
			// explicitly.
			dispose();
			setIsConnected(false);
			setRepositories(new Map());
			socketRef.current = null;
		};
	}, [chatId, agentStatus]);

	return { repositories, isConnected, refresh };
}
