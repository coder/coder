import { useEffect, useRef, useState } from "react";
import { watchChatGit } from "#/api/api";
import type {
	WorkspaceAgentGitClientMessage,
	WorkspaceAgentGitServerMessage,
	WorkspaceAgentRepoChanges,
	WorkspaceAgentStatus,
} from "#/api/typesGenerated";

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

	const sendMessage = (msg: WorkspaceAgentGitClientMessage) => {
		const socket = socketRef.current;
		if (socket && socket.readyState === WebSocket.OPEN) {
			socket.send(JSON.stringify(msg));
		}
	};

	const refresh = () => {
		sendMessage({ type: "refresh" });
	};

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
				// Ignore open events from superseded connections.
				if (socketRef.current !== socket) {
					return;
				}
				setIsConnected(true);
				reconnectAttemptRef.current = 0;
			});

			socket.addEventListener("message", (event) => {
				// Ignore messages from superseded connections.
				if (socketRef.current !== socket) {
					return;
				}
				let data: WorkspaceAgentGitServerMessage;
				try {
					data = JSON.parse(
						String(event.data),
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

			// Note: WebSocket "error" events are always followed by a "close"
			// event, so reconnection is handled here.
			socket.addEventListener("close", () => {
				// Ignore close events from superseded connections.
				if (socketRef.current !== socket) {
					return;
				}
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
