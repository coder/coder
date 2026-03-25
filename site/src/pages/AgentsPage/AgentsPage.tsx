import { useAuthenticated } from "hooks";
import { useDashboard } from "modules/dashboard/useDashboard";
import { type FC, useEffect, useRef, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { createReconnectingWebSocket } from "utils/reconnectingWebSocket";
import { API, watchChats } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import {
	archiveChat,
	cancelChatListQueries,
	chatDiffContentsKey,
	chatKey,
	chatModelConfigs,
	chatModels,
	infiniteChats,
	invalidateChatListQueries,
	prependToInfiniteChatsCache,
	readInfiniteChatsCache,
	unarchiveChat,
	updateInfiniteChatsCache,
} from "#/api/queries/chats";
import { workspaceById } from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { AgentsPageView } from "./AgentsPageView";
import { emptyInputStorageKey } from "./components/AgentCreateForm";
import { maybePlayChime } from "./components/AgentDetail/useAgentChime";
import { useAgentsPageKeybindings } from "./hooks/useAgentsPageKeybindings";
import { useAgentsPWA } from "./hooks/useAgentsPWA";
import {
	resolveArchiveAndDeleteAction,
	shouldNavigateAfterArchive,
} from "./utils/agentWorkspaceUtils";
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
		mutationFn: async ({
			chatId,
			workspaceId,
		}: {
			chatId: string;
			workspaceId: string;
		}) => {
			await API.experimental.updateChat(chatId, { archived: true });
			await API.deleteWorkspace(workspaceId);
			return { chatId, workspaceId };
		},
		onSuccess: async ({ chatId }) => {
			clearChatErrorReason(chatId);
			await invalidateChatListQueries(queryClient);
			await queryClient.invalidateQueries({
				queryKey: chatKey(chatId),
				exact: true,
			});
		},
		onError: (error) => {
			toast.error(getErrorMessage(error, "Failed to archive agent."));
		},
	});
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
	const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);
	const [chatErrorReasons, setChatErrorReasons] = useState<
		Record<string, ChatDetailError>
	>({});
	const catalogModelOptions = getModelOptionsFromConfigs(
		chatModelConfigsQuery.data,
		chatModelsQuery.data,
	);
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
	const requestArchiveAgent = (chatId: string) => {
		if (!isArchiving) {
			archiveAgentMutation.mutate(chatId);
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
							const activeChatId = activeChatIDRef.current;
							if (
								shouldNavigateAfterArchive(
									activeChatId,
									chatId,
									// Read root_chat_id from the per-chat
									// cache, which survives WebSocket eviction
									// of sub-agents (only the parent's chatKey
									// is removed). Must be read at settle time
									// so it reflects the user's current location.
									activeChatId
										? queryClient.getQueryData<TypesGen.Chat>(
												chatKey(activeChatId),
											)?.root_chat_id
										: undefined,
								)
							) {
								navigate("/agents");
							}
						},
					},
				);
			} else {
				setPendingArchiveAndDelete({ chatId, workspaceId });
			}
		} catch {
			toast.error("Failed to look up workspace for deletion.");
		}
	};
	const handleConfirmArchiveAndDelete = () => {
		if (pendingArchiveAndDelete && !isArchiving) {
			const { chatId: archivedChatId } = pendingArchiveAndDelete;
			archiveAndDeleteMutation.mutate(pendingArchiveAndDelete, {
				onSettled: () => {
					setPendingArchiveAndDelete(null);
					const activeChatId = activeChatIDRef.current;
					if (
						shouldNavigateAfterArchive(
							activeChatId,
							archivedChatId,
							activeChatId
								? queryClient.getQueryData<TypesGen.Chat>(chatKey(activeChatId))
										?.root_chat_id
								: undefined,
						)
					) {
						navigate("/agents");
					}
				},
			});
		}
	};
	const requestUnarchiveAgent = (chatId: string) => {
		unarchiveAgentMutation.mutate(chatId);
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

	// Track the active chat ID in a ref so the watchChats
	// WebSocket handler can read it without re-subscribing on
	// every navigation.
	const activeChatIDRef = useRef(agentId);
	useEffect(() => {
		activeChatIDRef.current = agentId;
	});

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
					void cancelChatListQueries(queryClient);
					void queryClient.cancelQueries({
						queryKey: chatKey(updatedChat.id),
						exact: true,
					});

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
								if (
									nextStatus === c.status &&
									nextTitle === c.title &&
									diffStatusEqual(nextDiffStatus, c.diff_status) &&
									nextWorkspaceId === c.workspace_id
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
							// unnecessary re-renders of AgentDetail during
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
				onToggleSidebarCollapsed={handleToggleSidebarCollapsed}
				isAgentsAdmin={isAgentsAdmin}
				hasNextPage={chatsQuery.hasNextPage}
				onLoadMore={() => void chatsQuery.fetchNextPage()}
				isFetchingNextPage={chatsQuery.isFetchingNextPage}
				archivedFilter={archivedFilter}
				onArchivedFilterChange={setArchivedFilter}
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
