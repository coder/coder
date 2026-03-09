import { watchChatGit } from "api/api";
import type {
	WorkspaceAgentGitClientMessage,
	WorkspaceAgentGitServerMessage,
	WorkspaceAgentRepoChanges,
	WorkspaceAgentStatus,
} from "api/typesGenerated";
import { useCallback, useEffect, useRef, useState } from "react";

interface UseGitWatcherOptions {
	chatId: string | undefined;
	agentStatus: WorkspaceAgentStatus | undefined;
}

interface UseGitWatcherResult {
	/** Current repo state, keyed by repo root path. */
	repositories: ReadonlyMap<string, WorkspaceAgentRepoChanges>;
	/** Whether the WebSocket is currently connected. */
	isConnected: boolean;
	/** Send a refresh request. */
	refresh: () => void;
}

const MAX_BACKOFF_MS = 30_000;

export function useGitWatcher({
	chatId,
	agentStatus,
}: UseGitWatcherOptions): UseGitWatcherResult {
	const [repositories, setRepositories] = useState<
		ReadonlyMap<string, WorkspaceAgentRepoChanges>
	>(new Map());
	const [isConnected, setIsConnected] = useState(false);

	const socketRef = useRef<WebSocket | null>(null);
	const reconnectAttemptRef = useRef(0);
	const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	// Track whether we've been disposed to avoid reconnecting after unmount.
	const disposedRef = useRef(false);

	const sendMessage = useCallback((msg: WorkspaceAgentGitClientMessage) => {
		const socket = socketRef.current;
		if (socket && socket.readyState === WebSocket.OPEN) {
			socket.send(JSON.stringify(msg));
		}
	}, []);

	const refresh = useCallback(() => {
		sendMessage({ type: "refresh" });
	}, [sendMessage]);

	useEffect(() => {
		if (!chatId || agentStatus !== "connected") {
			return;
		}

		disposedRef.current = false;

		function connect() {
			if (disposedRef.current) {
				return;
			}

			const socket = watchChatGit(chatId!);
			socketRef.current = socket;

			socket.addEventListener("open", () => {
				setIsConnected(true);
				reconnectAttemptRef.current = 0;
			});

			socket.addEventListener("message", (event) => {
				try {
					const data = JSON.parse(
						String(event.data),
					) as WorkspaceAgentGitServerMessage;

					if (data.type === "changes" && data.repositories) {
						setRepositories((prev) => {
							const next = new Map(prev);
							for (const repo of data.repositories!) {
								if (repo.removed) {
									next.delete(repo.repo_root);
								} else {
									next.set(repo.repo_root, repo);
								}
							}
							return next;
						});
					} else if (data.type === "error") {
						console.warn("[useGitWatcher] server error:", data.message);
					}
				} catch {
					// Ignore unparsable messages.
				}
			});

			// Note: WebSocket "error" events are always followed by a "close"
			// event, so reconnection is handled here.
			socket.addEventListener("close", () => {
				setIsConnected(false);
				socketRef.current = null;

				if (disposedRef.current) {
					return;
				}

				// Reconnect with exponential backoff.
				const attempt = reconnectAttemptRef.current;
				const delay = Math.min(1000 * 2 ** attempt, MAX_BACKOFF_MS);
				reconnectAttemptRef.current = attempt + 1;
				reconnectTimerRef.current = setTimeout(connect, delay);
			});
		}

		connect();

		return () => {
			disposedRef.current = true;
			if (reconnectTimerRef.current !== null) {
				clearTimeout(reconnectTimerRef.current);
				reconnectTimerRef.current = null;
			}
			if (socketRef.current) {
				socketRef.current.close();
				socketRef.current = null;
			}
			setIsConnected(false);
			setRepositories(new Map());
			reconnectAttemptRef.current = 0;
		};
	}, [chatId, agentStatus]);

	return { repositories, isConnected, refresh };
}
