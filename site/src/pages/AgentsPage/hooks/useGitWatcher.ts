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
	/**
	 * Repo roots known to the watcher that have shown a non-empty
	 * unified_diff at least once since the current chat was selected.
	 * Used by GitPanel to keep a tab visible after the tree goes clean,
	 * instead of hiding it the moment the diff empties out (which
	 * causes a visible flip when the agent edits a file and then
	 * reverts it).
	 *
	 * Entries are evicted when the watcher reports `removed: true` for
	 * the repo, and the entire set is cleared when `chatId` changes.
	 * It is preserved across reconnects and transient agentStatus
	 * flaps on the same chat. Consumers should intersect this with
	 * `repositories` before rendering, since a removed repo is dropped
	 * from both but a not-yet-known repo can appear in one without the
	 * other.
	 */
	everDirty: ReadonlySet<string>;
	/**
	 * Timestamp of the most recent scan observed from the server, or
	 * undefined if no scan has been received yet. Consumers can render
	 * this as a "checked Ns ago" affordance so users know how stale the
	 * local view is.
	 */
	lastCheckedAt: Date | undefined;
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
	const [everDirty, setEverDirty] = useState<ReadonlySet<string>>(
		() => new Set(),
	);
	const [lastCheckedAt, setLastCheckedAt] = useState<Date | undefined>(
		undefined,
	);
	const [isConnected, setIsConnected] = useState(false);

	const socketRef = useRef<WebSocket | null>(null);
	// Reset everDirty when chatId changes, using the official React
	// pattern for "adjust state on prop change" (a state mirror of the
	// prop + a setState during render that bails out when unchanged).
	// See https://react.dev/reference/react/useState#storing-information-from-previous-renders.
	// An agentStatus flap on the same chat must not clear everDirty,
	// so it is intentionally not part of this comparison.
	const [lastChatId, setLastChatId] = useState<string | undefined>(chatId);
	if (lastChatId !== chatId) {
		setLastChatId(chatId);
		setEverDirty((prev) => (prev.size === 0 ? prev : new Set()));
	}

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

					if (data.type === "changes") {
						if (data.scanned_at) {
							const parsed = new Date(data.scanned_at);
							if (!Number.isNaN(parsed.getTime())) {
								setLastCheckedAt(parsed);
							}
						}
						if (data.repositories) {
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
							setEverDirty((prev) => {
								let changed = false;
								const next = new Set(prev);
								for (const repo of data.repositories!) {
									if (repo.removed) {
										if (next.delete(repo.repo_root)) {
											changed = true;
										}
									} else if (repo.unified_diff && !next.has(repo.repo_root)) {
										next.add(repo.repo_root);
										changed = true;
									}
								}
								return changed ? next : prev;
							});
						}
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
			// explicitly. `everDirty` is intentionally preserved
			// across reconnects and brief agentStatus flaps; it is
			// cleared only when chatId changes (via the `lastChatId`
			// mirror-state check during render, above).
			dispose();
			setIsConnected(false);
			setRepositories(new Map());
			setLastCheckedAt(undefined);
			socketRef.current = null;
		};
	}, [chatId, agentStatus]);

	return { repositories, everDirty, lastCheckedAt, isConnected, refresh };
}
