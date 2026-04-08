import { type FC, useEffect, useRef, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { API, watchChats } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import {
	archiveChat,
	cancelChatListRefetches,
	chatDiffContentsKey,
	chatKey,
	chatModelConfigs,
	chatModels,
	chatsByWorkspaceKeyPrefix,
	infiniteChats,
	invalidateChatListQueries,
	pinChat,
	prependToInfiniteChatsCache,
	readInfiniteChatsCache,
	regenerateChatTitle,
	reorderPinnedChat,
	unarchiveChat,
	unpinChat,
	updateInfiniteChatsCache,
} from "#/api/queries/chats";
import { workspaceById } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import { AgentsPageView } from "./AgentsPageView";
import { emptyInputStorageKey } from "./components/AgentCreateForm";
import { useAgentsPageKeybindings } from "./hooks/useAgentsPageKeybindings";
import { useAgentsPWA } from "./hooks/useAgentsPWA";
import {
	archiveChatAndDeleteWorkspace,
	resolveArchiveAndDeleteAction,
	shouldNavigateAfterArchive,
} from "./utils/agentWorkspaceUtils";
import { maybePlayChime } from "./utils/chime";
import { getModelOptionsFromConfigs } from "./utils/modelOptions";
import {
	type ChatDetailError,
	chatDetailErrorsEqual,
} from "./utils/usageLimitMessage";

// Type guard for SSE events from the chat list watch endpoint.
// Shallow-compare two ChatDiffStatus objects by their meaningful
// fields, ignoring refreshed_at/stale_at which change on every poll.
function diffStatusEqual(
	a: TypesGen.ChatDiffStatus | undefined,
	b: TypesGen.ChatDiffStatus | undefined,
): boolean {
	if (a === b) return true;
	if (!a || !b) return false;
	return (
		a.url === b.url &&
		a.pull_request_state === b.pull_request_state &&
		a.pull_request_title === b.pull_request_title &&
		a.pull_request_draft === b.pull_request_draft &&
		a.changes_requested === b.changes_requested &&
		a.additions === b.additions &&
		a.deletions === b.deletions &&
		a.changed_files === b.changed_files &&
		a.pr_number === b.pr_number &&
		a.approved === b.approved &&
		a.commits === b.commits
	);
}

function isChatListSSEEvent(
	data: unknown,
): data is { kind: string; chat: TypesGen.Chat } {
	if (typeof data !== "object" || data === null) return false;
	const obj = data as Record<string, unknown>;
	return (
		typeof obj.kind === "string" &&
		typeof obj.chat === "object" &&
		obj.chat !== null &&
		"id" in obj.chat
	);
}

export type { AgentsOutletContext } from "./AgentsPageView";

const AgentsPage: FC = () => {
	useAgentsPWA();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const { agentId } = useParams();
	const { permissions } = useAuthenticated();
	const { appearance } = useDashboard();
	const isAgentsAdmin = permissions.editDeploymentConfig;

	const [archivedFilter, setArchivedFilter] = useState<"active" | "archived">(
		"active",
	);

	// The global CSS sets scrollbar-gutter: stable on <html> to prevent
	// layout shift on pages that toggle scrollbars. The agents page
	// uses its own internal scroll containers so the reserved gutter
	// space is unnecessary and wastes horizontal room.
	//
	// Removing the gutter requires three things:
	//
	// 1. overflow:hidden on both <html> and <body> so neither element
	//    can produce a scrollbar.
	// 2. scrollbar-gutter:auto on <html> so the browser stops
	//    reserving space for a scrollbar that will never appear.
	//    This is what makes react-remove-scroll-bar measure a gap of
	//    0 when a Radix dropdown opens, so it injects no padding or
	//    margin compensation.
	// 3. An injected <style> that overrides the global
	//    `overflow-y: scroll !important` on body[data-scroll-locked].
	//    Without this, opening any Radix dropdown would force a
	//    scrollbar onto <body>, re-introducing the layout shift.
	useEffect(() => {
		const html = document.documentElement;
		const body = document.body;

		const prevHtmlOverflow = html.style.overflow;
		const prevHtmlScrollbarGutter = html.style.scrollbarGutter;
		const prevBodyOverflow = body.style.overflow;

		html.style.overflow = "hidden";
		html.style.scrollbarGutter = "auto";
		body.style.overflow = "hidden";

		const style = document.createElement("style");
		style.textContent =
			"html body[data-scroll-locked] { overflow-y: hidden !important; }";
		document.head.appendChild(style);

		return () => {
			html.style.overflow = prevHtmlOverflow;
			html.style.scrollbarGutter = prevHtmlScrollbarGutter;
			body.style.overflow = prevBodyOverflow;
			style.remove();
		};
	}, []);

	const chatsQuery = useInfiniteQuery(
		infiniteChats({ archived: archivedFilter === "archived" }),
	);
	// Model queries are kept here for the sidebar, which displays
	// model info alongside each chat. Child routes that need models
	// subscribe to the same queries independently — react-query
	// deduplicates the requests.
	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const [chatErrorReasons, setChatErrorReasons] = useState<
		Record<string, ChatDetailError>
	>({});
	const setChatErrorReason = (chatId: string, reason: ChatDetailError) => {
		const trimmedMessage = reason.message.trim();
		if (!chatId || !trimmedMessage) {
			return;
		}
		const nextReason: ChatDetailError = {
			...reason,
			message: trimmedMessage,
		};
		setChatErrorReasons((current) => {
			const existing = current[chatId];
			if (chatDetailErrorsEqual(existing, nextReason)) {
				return current;
			}
			return {
				...current,
				[chatId]: nextReason,
			};
		});
	};
	const clearChatErrorReason = (chatId: string) => {
		if (!chatId) {
			return;
		}
		setChatErrorReasons((current) => {
			if (!(chatId in current)) {
				return current;
			}
			const next = { ...current };
			delete next[chatId];
			return next;
		});
	};

	const archiveChatBase = archiveChat(queryClient);
	const archiveAgentMutation = useMutation({
		...archiveChatBase,
		onSuccess: (_data, chatId) => {
			clearChatErrorReason(chatId);
		},
		onError: (error, chatId, context) => {
			archiveChatBase.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to archive agent."));
		},
	});
	const archiveAndDeleteMutation = useMutation({
		mutationFn: ({
			chatId,
			workspaceId,
		}: {
			chatId: string;
			workspaceId: string;
		}) =>
			archiveChatAndDeleteWorkspace(
				chatId,
				workspaceId,
				(id) => API.experimental.updateChat(id, { archived: true }),
				(id) => API.deleteWorkspace(id),
			),
		onSuccess: async ({ chatId }) => {
			clearChatErrorReason(chatId);
			await invalidateChatListQueries(queryClient);
			await queryClient.invalidateQueries({
				queryKey: chatKey(chatId),
				exact: true,
			});
			await queryClient.invalidateQueries({
				queryKey: chatsByWorkspaceKeyPrefix,
			});
		},
		onError: (error) => {
			toast.error(
				getErrorMessage(error, "Failed to archive and delete workspace."),
			);
		},
	});
	const [pendingArchiveChatId, setPendingArchiveChatId] = useState<
		string | null
	>(null);
	const [pendingArchiveAndDelete, setPendingArchiveAndDelete] = useState<{
		chatId: string;
		workspaceId: string;
	} | null>(null);
	const unarchiveChatBase = unarchiveChat(queryClient);
	const unarchiveAgentMutation = useMutation({
		...unarchiveChatBase,
		onError: (error, chatId, context) => {
			unarchiveChatBase.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to unarchive agent."));
		},
	});
	const pinChatBase = pinChat(queryClient);
	const pinAgentMutation = useMutation({
		...pinChatBase,
		onError: (error, chatId, context) => {
			pinChatBase.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to pin agent."));
		},
	});
	const unpinChatBase = unpinChat(queryClient);
	const unpinAgentMutation = useMutation({
		...unpinChatBase,
		onError: (error, chatId, context) => {
			unpinChatBase.onError(error, chatId, context);
			toast.error(getErrorMessage(error, "Failed to unpin agent."));
		},
	});
	const reorderPinnedChatMutation = useMutation({
		...reorderPinnedChat(queryClient),
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to reorder pinned agents."));
		},
	});
	const regenerateTitleMutation = useMutation({
		...regenerateChatTitle(queryClient),
		onError: (error: unknown) => {
			toast.error(getErrorMessage(error, "Failed to generate new title."));
		},
	});
	const regeneratingTitleChatIdsRef = useRef<ReadonlySet<string>>(new Set());
	const [regeneratingTitleChatIds, setRegeneratingTitleChatIds] = useState<
		readonly string[]
	>([]);
	const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);
	const catalogModelOptions = getModelOptionsFromConfigs(
		chatModelConfigsQuery.data,
		chatModelsQuery.data,
	);
	const chatList = chatsQuery.data?.pages.flat() ?? [];
	const isArchiving =
		archiveAgentMutation.isPending || archiveAndDeleteMutation.isPending;
	const archivingChatId =
		(archiveAgentMutation.isPending
			? archiveAgentMutation.variables
			: undefined) ??
		(archiveAndDeleteMutation.isPending
			? archiveAndDeleteMutation.variables?.chatId
			: undefined);
	const isActiveChat = (chat: TypesGen.Chat | undefined) =>
		chat?.status === "pending" || chat?.status === "running";
	const requestArchiveAgent = (chatId: string) => {
		if (isArchiving) {
			return;
		}
		const chat =
			queryClient.getQueryData<TypesGen.Chat>(chatKey(chatId)) ??
			chatList.find((candidate) => candidate.id === chatId);
		if (chat === undefined || isActiveChat(chat)) {
			setPendingArchiveChatId(chatId);
			return;
		}
		archiveAgentMutation.mutate(chatId);
	};
	const handleConfirmArchiveAgent = () => {
		if (!pendingArchiveChatId || isArchiving) {
			return;
		}
		archiveAgentMutation.mutate(pendingArchiveChatId, {
			onSettled: () => {
				setPendingArchiveChatId(null);
			},
		});
	};

	// Track the active chat ID in a ref so the watchChats
	// WebSocket handler can read it without re-subscribing
	// on every navigation.
	const activeChatIDRef = useRef(agentId);
	const navigateAfterArchive = (archivedChatId: string) => {
		const activeChatId = activeChatIDRef.current;
		if (
			shouldNavigateAfterArchive(
				activeChatId,
				archivedChatId,
				// Read root_chat_id from the per-chat cache, which
				// survives WebSocket eviction of sub-agents (only the
				// parent's chatKey is removed). This must be read at
				// callback time so it reflects the user's current
				// location.
				activeChatId
					? queryClient.getQueryData<TypesGen.Chat>(chatKey(activeChatId))
							?.root_chat_id
					: undefined,
			)
		) {
			navigate("/agents");
		}
	};

	const requestArchiveAndDeleteWorkspace = async (
		chatId: string,
		workspaceId: string,
	) => {
		if (isArchiving) {
			return;
		}
		try {
			const action = await resolveArchiveAndDeleteAction(
				() => queryClient.fetchQuery(workspaceById(workspaceId)),
				() =>
					readInfiniteChatsCache(queryClient)?.find((c) => c.id === chatId)
						?.created_at,
			);
			if (action === "proceed") {
				archiveAndDeleteMutation.mutate(
					{ chatId, workspaceId },
					{
						onSettled: () => {
							navigateAfterArchive(chatId);
						},
					},
				);
			} else if (action === "archive-only") {
				// The workspace is already gone (404), so we skip the
				// running-agent confirmation dialog. That dialog warns
				// about interrupting a live workspace, which is moot
				// when the workspace no longer exists.
				archiveAgentMutation.mutate(chatId, {
					// Navigate only on success. The proceed/confirm paths
					// use onSettled because their pre-existing behavior
					// navigates regardless of delete outcome. This path
					// has no delete step, so a failed archive should not
					// redirect the user.
					onSuccess: () => {
						navigateAfterArchive(chatId);
					},
				});
			} else {
				setPendingArchiveAndDelete({ chatId, workspaceId });
			}
		} catch (error) {
			toast.error(
				getErrorMessage(error, "Failed to look up workspace for deletion."),
			);
		}
	};
	const handleConfirmArchiveAndDelete = () => {
		if (pendingArchiveAndDelete && !isArchiving) {
			const { chatId: archivedChatId } = pendingArchiveAndDelete;
			archiveAndDeleteMutation.mutate(pendingArchiveAndDelete, {
				onSettled: () => {
					setPendingArchiveAndDelete(null);
					navigateAfterArchive(archivedChatId);
				},
			});
		}
	};
	const requestUnarchiveAgent = (chatId: string) => {
		unarchiveAgentMutation.mutate(chatId);
	};
	const requestPinAgent = (chatId: string) => {
		pinAgentMutation.mutate(chatId);
	};
	const requestUnpinAgent = (chatId: string) => {
		unpinAgentMutation.mutate(chatId);
	};
	const requestReorderPinnedAgent = (chatId: string, pinOrder: number) => {
		reorderPinnedChatMutation.mutate({ chatId, pinOrder });
	};
	const addRegeneratingTitleChatId = (chatId: string) => {
		if (!chatId || regeneratingTitleChatIdsRef.current.has(chatId)) {
			return false;
		}
		const next = new Set(regeneratingTitleChatIdsRef.current);
		next.add(chatId);
		regeneratingTitleChatIdsRef.current = next;
		setRegeneratingTitleChatIds(Array.from(next));
		return true;
	};
	const removeRegeneratingTitleChatId = (chatId: string) => {
		if (!regeneratingTitleChatIdsRef.current.has(chatId)) {
			return;
		}
		const next = new Set(regeneratingTitleChatIdsRef.current);
		next.delete(chatId);
		regeneratingTitleChatIdsRef.current = next;
		setRegeneratingTitleChatIds(Array.from(next));
	};
	const requestRegenerateTitle = (chatId: string) => {
		if (!addRegeneratingTitleChatId(chatId)) {
			return;
		}
		void regenerateTitleMutation
			.mutateAsync(chatId)
			.catch(() => {
				// The shared mutation onError already reports the failure.
			})
			.finally(() => {
				removeRegeneratingTitleChatId(chatId);
			});
	};
	const handleToggleSidebarCollapsed = () =>
		setIsSidebarCollapsed((prev) => !prev);

	const handleNewAgent = () => {
		// Only clear the draft when the user is already on the empty
		// state and explicitly requests a blank slate.  When navigating
		// back from a conversation the existing draft is preserved.
		if (!agentId) {
			localStorage.removeItem(emptyInputStorageKey);
		}
		navigate("/agents");
	};

	useEffect(() => {
		activeChatIDRef.current = agentId;
	});

	// Optimistically clear the unread indicator for the active
	// chat. The server marks chats as read on stream connect
	// and disconnect, but the list cache is not refetched until
	// window focus. Without this, navigating away from a chat
	// causes its cached has_unread to reappear as a stale dot.
	useEffect(() => {
		if (!agentId) {
			return;
		}
		updateInfiniteChatsCache(queryClient, (chats) => {
			let changed = false;
			const next = chats.map((c) => {
				if (c.id !== agentId || !c.has_unread) return c;
				changed = true;
				return { ...c, has_unread: false };
			});
			return changed ? next : chats;
		});
	}, [agentId, queryClient]);
	useEffect(() => {
		return createReconnectingWebSocket({
			connect() {
				const ws = watchChats();

				ws.addEventListener("message", (event) => {
					if (event.parseError) {
						console.warn("Failed to parse chat watch event:", event.parseError);
						return;
					}
					const sse = event.parsedMessage;
					if (sse?.type !== "data" || !sse.data) {
						return;
					}
					if (!isChatListSSEEvent(sse.data)) {
						return;
					}
					const chatEvent = sse.data;
					const updatedChat = chatEvent.chat;
					// Read the previous status from the infinite chat list
					// cache before we write the update below. The per-chat
					// query cache (chatKey) only exists for chats the user
					// has opened, so reading from the list cache ensures
					// prevStatus is available for background agents too.
					const prevStatus = readInfiniteChatsCache(queryClient)?.find(
						(c) => c.id === updatedChat.id,
					)?.status;
					// Only play the chime for top-level chats, not sub-agents.
					if (!updatedChat.parent_chat_id) {
						maybePlayChime(
							prevStatus,
							updatedChat.status,
							updatedChat.id,
							activeChatIDRef.current,
						);
					}

					if (chatEvent.kind === "deleted") {
						updateInfiniteChatsCache(queryClient, (chats) =>
							chats.filter(
								(c) =>
									c.id !== updatedChat.id && c.root_chat_id !== updatedChat.id,
							),
						);
						queryClient.removeQueries({
							queryKey: chatKey(updatedChat.id),
							exact: true,
						});
						return;
					}

					if (chatEvent.kind === "diff_status_change") {
						// Only refetch the diff file contents — the chat's
						// diff_status field is already written into the
						// chatKey and infinite-list caches below.
						void queryClient.invalidateQueries({
							queryKey: chatDiffContentsKey(updatedChat.id),
							exact: true,
						});
					}
					// Scope field updates by event kind so that
					// status_change events (which may carry a stale title
					// snapshot from before async title generation
					// finished) don't clobber a title_change that already
					// landed.
					const isTitleEvent = chatEvent.kind === "title_change";
					const isStatusEvent = chatEvent.kind === "status_change";
					const isDiffStatusEvent = chatEvent.kind === "diff_status_change";

					// Cancel in-flight list and per-chat refetches so
					// they cannot overwrite the cache update below with
					// stale server data. This matters when a title_change
					// event races with a refetch triggered by
					// createChat.onSuccess or the onOpen invalidation:
					// the refetch may have been issued before the async
					// title generation finished, so its response carries
					// the fallback title.
					void cancelChatListRefetches(queryClient);
					// Only cancel a per-chat refetch when the cache
					// already has data. Cancelling a first-time fetch
					// reverts the query to pending/idle with no data
					// and no retry, which AgentChatPage shows as
					// "Chat not found".
					if (queryClient.getQueryData(chatKey(updatedChat.id))) {
						void queryClient.cancelQueries({
							queryKey: chatKey(updatedChat.id),
							exact: true,
						});
					}

					// For "created" events, use a cross-page existence
					// check and prepend only to the first page.
					// updateInfiniteChatsCache runs the updater per
					// page, so a naive prepend would duplicate the
					// chat into every loaded page.
					if (chatEvent.kind === "created") {
						prependToInfiniteChatsCache(queryClient, updatedChat);
					} else {
						updateInfiniteChatsCache(queryClient, (chats) => {
							let didUpdate = false;
							const nextChats = chats.map((c) => {
								if (c.id !== updatedChat.id) return c;
								const nextStatus = isStatusEvent
									? updatedChat.status
									: c.status;
								const nextTitle = isTitleEvent ? updatedChat.title : c.title;
								const nextDiffStatus = isDiffStatusEvent
									? updatedChat.diff_status
									: c.diff_status;
								const nextWorkspaceId =
									updatedChat.workspace_id ?? c.workspace_id;
								const nextUpdatedAt =
									c.updated_at > updatedChat.updated_at
										? c.updated_at
										: updatedChat.updated_at;
								// The server's pubsub path does not compute
								// has_unread (it always sends false). For
								// status_change events on non-active chats,
								// optimistically mark as unread since the
								// assistant produced new output.
								const nextHasUnread =
									isStatusEvent && updatedChat.id !== activeChatIDRef.current
										? true
										: c.has_unread;
								if (
									nextStatus === c.status &&
									nextTitle === c.title &&
									diffStatusEqual(nextDiffStatus, c.diff_status) &&
									nextWorkspaceId === c.workspace_id &&
									nextHasUnread === c.has_unread
								) {
									return c;
								}
								didUpdate = true;
								return {
									...c,
									status: nextStatus,
									title: nextTitle,
									diff_status: nextDiffStatus,
									workspace_id: nextWorkspaceId,
									updated_at: nextUpdatedAt,
									has_unread: nextHasUnread,
								};
							});
							return didUpdate ? nextChats : chats;
						});
					}
					queryClient.setQueryData<TypesGen.Chat | undefined>(
						chatKey(updatedChat.id),
						(previousChat) => {
							if (!previousChat) {
								return previousChat;
							}
							// Only create a new object if a field actually
							// changed. Returning the same reference prevents
							// react-query from notifying subscribers, avoiding
							// unnecessary re-renders of AgentChatPage during
							// streaming when repeated status_change events
							// carry the same "running" status.
							const nextStatus = isStatusEvent
								? updatedChat.status
								: previousChat.status;
							const nextTitle = isTitleEvent
								? updatedChat.title
								: previousChat.title;
							const nextDiffStatus = isDiffStatusEvent
								? updatedChat.diff_status
								: previousChat.diff_status;
							const nextWorkspaceId =
								updatedChat.workspace_id ?? previousChat.workspace_id;
							const nextUpdatedAt =
								previousChat.updated_at > updatedChat.updated_at
									? previousChat.updated_at
									: updatedChat.updated_at;

							if (
								nextStatus === previousChat.status &&
								nextTitle === previousChat.title &&
								diffStatusEqual(nextDiffStatus, previousChat.diff_status) &&
								nextWorkspaceId === previousChat.workspace_id
							) {
								return previousChat;
							}
							return {
								...previousChat,
								status: nextStatus,
								title: nextTitle,
								diff_status: nextDiffStatus,
								workspace_id: nextWorkspaceId,
								updated_at: nextUpdatedAt,
							};
						},
					);
				});
				return ws;
			},
			onOpen() {
				void invalidateChatListQueries(queryClient);
			},
		});
	}, [queryClient]);

	useAgentsPageKeybindings({
		onNewAgent: handleNewAgent,
	});

	// Fetch workspace name for the confirmation dialog. Only
	// enabled when pendingArchiveAndDelete is set (i.e. the
	// resolve step determined confirmation is needed). The
	// workspace data is usually already cached from the
	// fetchQuery in requestArchiveAndDeleteWorkspace.
	const pendingWorkspaceQuery = useQuery({
		...workspaceById(pendingArchiveAndDelete?.workspaceId ?? ""),
		enabled: Boolean(pendingArchiveAndDelete?.workspaceId),
	});
	const pendingWorkspaceName = pendingWorkspaceQuery.data?.name ?? "";

	const deleteDialogOpen =
		pendingArchiveAndDelete !== null && Boolean(pendingWorkspaceName);

	return (
		<>
			<AgentsPageView
				agentId={agentId}
				chatList={chatList}
				catalogModelOptions={catalogModelOptions}
				modelConfigs={chatModelConfigsQuery.data ?? []}
				logoUrl={appearance.logo_url}
				handleNewAgent={handleNewAgent}
				isCreating={false}
				isArchiving={isArchiving}
				archivingChatId={archivingChatId}
				isChatsLoading={chatsQuery.isLoading}
				chatsLoadError={chatsQuery.error}
				onRetryChatsLoad={() => void chatsQuery.refetch()}
				onCollapseSidebar={() => setIsSidebarCollapsed(true)}
				isSidebarCollapsed={isSidebarCollapsed}
				onExpandSidebar={() => setIsSidebarCollapsed(false)}
				chatErrorReasons={chatErrorReasons}
				setChatErrorReason={setChatErrorReason}
				clearChatErrorReason={clearChatErrorReason}
				requestArchiveAgent={requestArchiveAgent}
				requestUnarchiveAgent={requestUnarchiveAgent}
				requestArchiveAndDeleteWorkspace={requestArchiveAndDeleteWorkspace}
				requestPinAgent={requestPinAgent}
				requestUnpinAgent={requestUnpinAgent}
				requestReorderPinnedAgent={requestReorderPinnedAgent}
				onRegenerateTitle={requestRegenerateTitle}
				regeneratingTitleChatIds={regeneratingTitleChatIds}
				onToggleSidebarCollapsed={handleToggleSidebarCollapsed}
				isAgentsAdmin={isAgentsAdmin}
				hasNextPage={chatsQuery.hasNextPage}
				onLoadMore={() => void chatsQuery.fetchNextPage()}
				isFetchingNextPage={chatsQuery.isFetchingNextPage}
				archivedFilter={archivedFilter}
				onArchivedFilterChange={setArchivedFilter}
			/>
			<ConfirmDialog
				open={pendingArchiveChatId !== null}
				onClose={() => setPendingArchiveChatId(null)}
				onConfirm={handleConfirmArchiveAgent}
				type="delete"
				confirmText="Archive"
				confirmLoading={archiveAgentMutation.isPending}
				title="Archive agent?"
				description="This agent is currently running. Archiving it will interrupt the current run."
			/>
			<DeleteDialog
				key={pendingWorkspaceName}
				isOpen={deleteDialogOpen}
				onConfirm={handleConfirmArchiveAndDelete}
				onCancel={() => setPendingArchiveAndDelete(null)}
				entity="workspace"
				name={pendingWorkspaceName}
				confirmLoading={archiveAndDeleteMutation.isPending}
				title="Archive agent & delete workspace"
				verb="Archiving and deleting"
				info="This will archive the agent and permanently delete the associated workspace and all its resources."
			/>
		</>
	);
};
export default AgentsPage;
