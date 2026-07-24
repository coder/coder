import { type FC, type RefObject, useEffect, useRef, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import {
	Outlet,
	useLocation,
	useNavigate,
	useParams,
	useSearchParams,
} from "react-router";
import { toast } from "sonner";
import { API, watchChats } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import {
	addChildToParentInCache,
	applyChatArchiveStateToCaches,
	archiveChat,
	cancelChatListRefetches,
	chatDiffContentsKey,
	chatKey,
	chatModelConfigs,
	chatModels,
	chatsByWorkspaceKeyPrefix,
	infiniteChats,
	invalidateChatListQueries,
	mergeWatchedChatIntoCaches,
	pinChat,
	prependToInfiniteChatsCache,
	proposeChatTitle,
	readInfiniteChatsCache,
	removeChildFromParentInCache,
	reorderPinnedChat,
	unarchiveChat,
	unpinChat,
	updateChatTitle,
	updateInfiniteChatsCache,
	userChatPersonalModelOverrides,
	userChatProviderConfigs,
} from "#/api/queries/chats";
import {
	invalidateWorkspaceMutationQueries,
	workspaceById,
	workspaceByIdKey,
} from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialogs/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "#/components/Dialogs/DeleteDialog/DeleteDialog";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	getDefaultOrganizationName,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { cn } from "#/utils/cn";
import { pageTitle } from "#/utils/page";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import { emptyInputStorageKey } from "./components/AgentCreateForm";
import {
	ChatsSidebar,
	isSettingsView,
	sidebarViewFromPath,
} from "./components/ChatsSidebar/ChatsSidebar";
import { ResizableChatsSidebarFrame } from "./components/ChatsSidebar/ResizableChatsSidebarFrame";
import { useAgentsPageKeybindings } from "./hooks/useAgentsPageKeybindings";
import { useAgentsPWA } from "./hooks/useAgentsPWA";
import { getAgentSidebarFilters } from "./utils/agentSidebarFilters";
import {
	ArchiveAndDeleteError,
	archiveChatAndDeleteWorkspace,
	notifyArchiveAndDeleteFailed,
	notifyDeleteQueueState,
	resolveArchiveAndDeleteAction,
	shouldNavigateAfterArchive,
} from "./utils/agentWorkspaceUtils";
import { maybePlayChime } from "./utils/chime";
import {
	getModelOptionsFromConfigs,
	providerInfoByIDFromUserConfigs,
} from "./utils/modelOptions";
import { clearPersistedRightPanelState } from "./utils/rightPanelTabStorage";
import { clearPersistedSidebarTabId } from "./utils/sidebarTabStorage";
import {
	type ChatDetailError,
	chatDetailErrorsEqual,
} from "./utils/usageLimitMessage";

export interface AgentsPageOutletContext {
	chatErrorReasons: Record<string, ChatDetailError>;
	setChatErrorReason: (chatId: string, reason: ChatDetailError) => void;
	clearChatErrorReason: (chatId: string) => void;
	requestArchiveAgent: (chatId: string) => void;
	requestUnarchiveAgent: (chatId: string) => void;
	requestArchiveAndDeleteWorkspace: (
		chatId: string,
		workspaceId: string,
	) => void;
	requestPinAgent: (chatId: string) => void;
	requestUnpinAgent: (chatId: string) => void;
	requestReorderPinnedAgent?: (chatId: string, pinOrder: number) => void;
	isArchiving: boolean;
	archivingChatId: string | undefined;
	onRenameTitle?: (chatId: string, title: string) => Promise<void>;
	/** Opens the shared rename dialog so both menus drive the same instance. */
	onOpenRenameDialog?: (chat: TypesGen.Chat) => void;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
	onExpandSidebar: () => void;
	onChatReady: () => void;
	/** Ref attached to the chat scroll container by AgentChatPage. */
	scrollContainerRef: RefObject<HTMLDivElement | null>;
}

const FILTER_MEMBERSHIP_EVENT_KINDS = new Set<TypesGen.ChatWatchEventKind>([
	"diff_status_change",
	"status_change",
]);

export const shouldInvalidateFilteredChatList = (
	chat: TypesGen.Chat,
	eventKind: TypesGen.ChatWatchEventKind,
): boolean =>
	!chat.parent_chat_id && FILTER_MEMBERSHIP_EVENT_KINDS.has(eventKind);

const AgentsPageLayout: FC = () => {
	useAgentsPWA();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();
	const [searchParams, setSearchParams] = useSearchParams();
	const { agentId } = useParams();
	const { permissions, user } = useAuthenticated();
	const { organizations } = useDashboard();
	const organizationName = getDefaultOrganizationName(organizations);
	const isAgentsAdmin = permissions.editDeploymentConfig;

	const [sidebarFilters, setSidebarFilters] = getAgentSidebarFilters(
		searchParams,
		setSearchParams,
	);
	const [isSearchDialogOpen, setIsSearchDialogOpen] = useState(false);

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

	const archivedFilter = sidebarFilters.archiveStatus === "archived";
	const chatStatusFilter =
		sidebarFilters.chatStatuses.length === 1
			? sidebarFilters.chatStatuses[0]
			: undefined;
	const chatsQuery = useInfiniteQuery(
		infiniteChats({
			archived: archivedFilter,
			prStatuses: sidebarFilters.prStatuses,
			chatStatus: chatStatusFilter,
			sources: sidebarFilters.sources,
		}),
	);
	// Model queries are kept here for the sidebar, which displays
	// model info alongside each chat. Child routes that need models
	// subscribe to the same queries independently, and react-query
	// deduplicates the requests.
	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const chatProviderConfigsQuery = useQuery(userChatProviderConfigs());
	const personalModelOverridesQuery = useQuery(
		userChatPersonalModelOverrides(),
	);
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
		onSuccess: (data, chatId) => {
			archiveChatBase.onSuccess(data, chatId);
			clearChatErrorReason(chatId);
			clearPersistedSidebarTabId(chatId);
			clearPersistedRightPanelState(chatId);
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
		onSuccess: ({ chatId, workspaceId, deleteBuild }) => {
			applyChatArchiveStateToCaches(queryClient, chatId, true);
			clearChatErrorReason(chatId);
			clearPersistedSidebarTabId(chatId);
			clearPersistedRightPanelState(chatId);
			void invalidateChatListQueries(queryClient);
			void queryClient.invalidateQueries({
				queryKey: chatKey(chatId),
				exact: true,
			});
			void queryClient.invalidateQueries({
				queryKey: chatsByWorkspaceKeyPrefix,
			});
			void invalidateWorkspaceMutationQueries(queryClient, {
				organizationName,
				username: user.username,
			});
			notifyDeleteQueueState(
				queryClient.getQueryData<TypesGen.Workspace>(
					workspaceByIdKey(workspaceId),
				),
				deleteBuild,
			);
		},
		onError: (error, { workspaceId }) => {
			notifyArchiveAndDeleteFailed(
				queryClient.getQueryData<TypesGen.Workspace>(
					workspaceByIdKey(workspaceId),
				),
				error,
				(path) => navigate(path),
			);
			// Archive failed after the delete already ran; refresh
			// workspace state so consumers see the deletion.
			if (error instanceof ArchiveAndDeleteError && error.step === "archive") {
				void invalidateWorkspaceMutationQueries(queryClient, {
					organizationName,
					username: user.username,
				});
			}
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
	const proposeTitleMutation = useMutation(proposeChatTitle(queryClient));
	const renameTitleMutation = useMutation({
		...updateChatTitle(queryClient),
		onError: (error: unknown) => {
			toast.error(getErrorMessage(error, "Failed to rename chat."));
		},
	});
	const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);
	const catalogModelOptions = getModelOptionsFromConfigs(
		chatModelConfigsQuery.data,
		chatModelsQuery.data,
		providerInfoByIDFromUserConfigs(chatProviderConfigsQuery.data),
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
	// A chat in any of these statuses has an in-flight run that
	// archiving would interrupt, so ask for confirmation first.
	const isActiveChat = (chat: TypesGen.Chat | undefined) =>
		chat?.status === "running" ||
		chat?.status === "interrupting" ||
		chat?.status === "requires_action";
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
			navigate({ pathname: "/agents", search: location.search });
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
				// We only need build_number 1 and 2 to recognise a
				// prebuild claim. The default page is newest-first; the
				// resolver degrades safely ("confirm") if those builds
				// aren't in the returned slice.
				() =>
					queryClient.fetchQuery({
						queryKey: [
							"workspaceBuilds",
							workspaceId,
							"archive-and-delete-resolver",
						],
						queryFn: () => API.getWorkspaceBuilds(workspaceId),
					}),
				() =>
					readInfiniteChatsCache(queryClient)?.find(
						(chat) => chat.id === chatId,
					)?.created_at,
			);
			if (action === "proceed") {
				archiveAndDeleteMutation.mutate(
					{ chatId, workspaceId },
					{
						onSuccess: () => {
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
				},
				onSuccess: () => {
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
	const requestProposeTitle = async (chatId: string): Promise<string> => {
		const result = await proposeTitleMutation.mutateAsync(chatId);
		return result.title;
	};
	const requestRenameTitle = async (chatId: string, title: string) => {
		await renameTitleMutation.mutateAsync({ chatId, title });
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
		navigate({ pathname: "/agents", search: location.search });
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
		void invalidateChatListQueries(queryClient);
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
					const chatEvent = event.parsedMessage;
					const updatedChat = chatEvent.chat;
					// The old membership is only available before the cache write below.
					const prevStatus = readInfiniteChatsCache(queryClient)?.find(
						(chat) => chat.id === updatedChat.id,
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
						// Drop the chat from the flat root list (root or
						// cascade via root_chat_id) and from any parent's
						// embedded children (individual child archive).
						updateInfiniteChatsCache(queryClient, (chats) =>
							chats.filter(
								(c) =>
									c.id !== updatedChat.id && c.root_chat_id !== updatedChat.id,
							),
						);
						removeChildFromParentInCache(queryClient, updatedChat.id);
						queryClient.removeQueries({
							queryKey: chatKey(updatedChat.id),
							exact: true,
						});
						return;
					}
					if (chatEvent.kind === "diff_status_change") {
						// Only refetch the diff file contents. The chat's
						// diff_status field is already written into the
						// chatKey and infinite-list caches below.
						void queryClient.invalidateQueries({
							queryKey: chatDiffContentsKey(updatedChat.id),
							exact: true,
						});
					}
					// Merge watch payloads by event kind so stale field
					// snapshots do not clobber fresher cached metadata.

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

					if (chatEvent.kind === "created") {
						if (updatedChat.parent_chat_id) {
							// Child chat: add to its parent's children
							// array. If the parent is not in any loaded
							// page, the child is silently dropped.
							addChildToParentInCache(
								queryClient,
								updatedChat,
								updatedChat.parent_chat_id,
							);
						} else {
							prependToInfiniteChatsCache(queryClient, updatedChat);
							void invalidateChatListQueries(queryClient);
						}
					} else {
						mergeWatchedChatIntoCaches(queryClient, updatedChat, {
							eventKind: chatEvent.kind,
							activeChatId: activeChatIDRef.current,
						});
						if (shouldInvalidateFilteredChatList(updatedChat, chatEvent.kind)) {
							void invalidateChatListQueries(queryClient);
						}
						if (chatEvent.kind === "context_dirty") {
							// The watch payload carries only the lightweight
							// context flags (the merge above applies them);
							// refetch the open chat to pull the pinned
							// resources the single-chat GET computes. Only the
							// active chat has an observer, so other chats are
							// merely marked stale.
							void queryClient.invalidateQueries({
								queryKey: chatKey(updatedChat.id),
								exact: true,
							});
						}
					}
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
		onToggleSearch: () => setIsSearchDialogOpen((open) => !open),
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

	// Mobile can't fit the sidebar nav and content side by side,
	// so we show one or the other depending on the route depth.
	const sidebarView = sidebarViewFromPath(location.pathname);
	const isSettingsPanel = isSettingsView(sidebarView);
	const isSettingsIndex = isSettingsPanel && !sidebarView.section;
	const isSettingsDetail = isSettingsPanel && Boolean(sidebarView.section);
	const isAnalytics = sidebarView.panel === "analytics";

	// The sidebar expects plain string error messages, but the outlet
	// context carries structured ChatDetailError objects.
	const sidebarChatErrorReasons = Object.fromEntries(
		Object.entries(chatErrorReasons).map(([chatId, error]) => [
			chatId,
			error.message,
		]),
	);

	const scrollContainerRef = useRef<HTMLDivElement | null>(null);

	// State for the shared rename-chat dialog. Lifted here so both the
	// sidebar menu and the chat top bar open the same dialog instance.
	const [chatPendingRename, setChatPendingRename] =
		useState<TypesGen.Chat | null>(null);

	const outletContextValue: AgentsPageOutletContext = {
		chatErrorReasons,
		setChatErrorReason,
		clearChatErrorReason,
		requestArchiveAgent,
		requestUnarchiveAgent,
		requestArchiveAndDeleteWorkspace,
		requestPinAgent,
		requestUnpinAgent,
		requestReorderPinnedAgent,
		isArchiving,
		archivingChatId,
		onOpenRenameDialog: setChatPendingRename,
		isSidebarCollapsed,
		onToggleSidebarCollapsed: handleToggleSidebarCollapsed,
		onExpandSidebar: () => setIsSidebarCollapsed(false),
		onChatReady: () => {},
		scrollContainerRef,
	};

	return (
		<>
			<div
				data-testid="agents-page-layout"
				className="flex h-full min-h-0 flex-col overflow-hidden bg-surface-primary sm:flex-row"
			>
				<title>{pageTitle("Agents")}</title>
				<ResizableChatsSidebarFrame
					className={cn(
						"sm:h-full sm:min-h-0 sm:border-b-0",
						agentId
							? "hidden sm:block shrink-0 h-[42dvh] min-h-[240px] border-b border-border-default"
							: isSettingsDetail || isAnalytics
								? "hidden sm:block shrink-0"
								: "order-2 sm:order-none flex-1 min-h-0 border-b border-border-default sm:flex-none sm:border-t-0 sm:border-b-0",
						isSidebarCollapsed && "sm:hidden",
					)}
				>
					<ChatsSidebar
						chats={chatList}
						currentUserId={user.id}
						chatErrorReasons={sidebarChatErrorReasons}
						modelOptions={catalogModelOptions}
						modelConfigs={chatModelConfigsQuery.data ?? []}
						onArchiveAgent={requestArchiveAgent}
						onUnarchiveAgent={requestUnarchiveAgent}
						onArchiveAndDeleteWorkspace={requestArchiveAndDeleteWorkspace}
						onPinAgent={requestPinAgent}
						onUnpinAgent={requestUnpinAgent}
						onReorderPinnedAgent={requestReorderPinnedAgent}
						onRenameTitle={requestRenameTitle}
						onProposeTitle={requestProposeTitle}
						chatPendingRename={chatPendingRename}
						onChatPendingRenameChange={setChatPendingRename}
						onBeforeNewAgent={handleNewAgent}
						isSearchDialogOpen={isSearchDialogOpen}
						onSearchDialogOpenChange={setIsSearchDialogOpen}
						isCreating={false}
						isArchiving={isArchiving}
						archivingChatId={archivingChatId}
						isLoading={chatsQuery.isLoading}
						loadError={chatsQuery.error}
						onRetryLoad={() => void chatsQuery.refetch()}
						hasNextPage={chatsQuery.hasNextPage}
						onLoadMore={() => void chatsQuery.fetchNextPage()}
						isFetchingNextPage={chatsQuery.isFetchingNextPage}
						sidebarFilters={sidebarFilters}
						onSidebarFiltersChange={setSidebarFilters}
						onCollapse={() => setIsSidebarCollapsed(true)}
						isPersonalModelOverridesEnabled={
							personalModelOverridesQuery.data?.enabled
						}
						isAdmin={isAgentsAdmin}
					/>
				</ResizableChatsSidebarFrame>
				<div
					data-testid="agents-main-panel"
					className={cn(
						"min-h-0 min-w-0 flex-1 flex-col bg-surface-primary",
						isSettingsIndex ? "hidden sm:flex" : "flex",
						!agentId &&
							!isSettingsDetail &&
							sidebarView.panel === "chats" &&
							"contents sm:flex sm:flex-1 sm:flex-col",
					)}
				>
					<Outlet context={outletContextValue} />
				</div>
			</div>
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
export default AgentsPageLayout;
