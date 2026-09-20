import { cn } from "cn";
import { type FC, useEffect, useRef, useState } from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQueries,
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
	applyWatchedChatArchived,
	applyWatchedChatCreatedOrUnarchived,
	archiveChat,
	cancelChatListRefetches,
	cancelChatTreeRefetches,
	cancelLoadedChatEntityRefetch,
	chatEntityKey,
	chatTree,
	countChatTreeDescendantsInCaches,
	infiniteChats,
	invalidateChatCostTree,
	invalidateChatDiffContents,
	invalidateChatEntity,
	invalidateChatListQueries,
	invalidateChatSearches,
	invalidateChatsByWorkspace,
	invalidateChatTreeQueries,
	mergeWatchedChatIntoCaches,
	pinChat,
	prependToInfiniteChatsCache,
	proposeChatTitle,
	readChatFromSidebarCaches,
	removeChatFromChatsByWorkspace,
	reorderPinnedChat,
	shouldInvalidateChatSearches,
	shouldInvalidateChatsByWorkspace,
	unarchiveChat,
	unpinChat,
	updateChatTitle,
	updateChatTreeCaches,
	updateInfiniteChatsCache,
	userChatPersonalModelOverrides,
} from "#/api/queries/chats";
import {
	invalidateWorkspaceMutationQueries,
	workspaceById,
	workspaceByIdKey,
} from "#/api/queries/workspaces";
import type * as TypesGen from "#/api/typesGenerated";
import { ConfirmDialog } from "#/components/Dialog/ConfirmDialog/ConfirmDialog";
import { DeleteDialog } from "#/components/Dialog/DeleteDialog/DeleteDialog";
import { useAuthenticated } from "#/hooks/useAuthenticated";
import {
	getDefaultOrganizationId,
	getDefaultOrganizationName,
	useDashboard,
} from "#/modules/dashboard/useDashboard";
import { canAccessCoderAgentsSettings } from "#/modules/permissions";
import { pageTitle } from "#/utils/page";
import { createReconnectingWebSocket } from "#/utils/reconnectingWebSocket";
import { emptyInputStorageKey } from "./components/AgentCreateForm";
import {
	type ChatDetailError,
	chatDetailErrorsEqual,
} from "./components/ChatConversation/chatError";
import { getChatCostTreeID } from "./components/ChatConversation/chatHelpers";
import { isActiveChatStatus } from "./components/ChatConversation/chatStore";
import {
	ChatsSidebar,
	isSettingsView,
	sidebarViewFromPath,
} from "./components/ChatsSidebar/ChatsSidebar";
import { ResizableChatsSidebarFrame } from "./components/ChatsSidebar/ResizableChatsSidebarFrame";
import type { ChatTreePanelData } from "./components/ChatsSidebar/tree/ChatTreePanel";
import { useAgentsPageKeybindings } from "./hooks/useAgentsPageKeybindings";
import { useAgentsPWA } from "./hooks/useAgentsPWA";
import { useOrganizationChatModels } from "./hooks/useOrganizationChatModels";
import { getAgentSidebarFilters } from "./utils/agentSidebarFilters";
import {
	archiveChatAndDeleteWorkspace,
	notifyArchiveAndDeleteFailed,
	notifyDeleteQueueState,
	resolveArchiveAndDeleteAction,
	shouldNavigateAfterArchive,
} from "./utils/agentWorkspaceUtils";
import { maybePlayChime } from "./utils/chime";
import type { NewChildChatLocationState } from "./utils/navigation";
import { clearPersistedRightPanelState } from "./utils/rightPanelTabStorage";
import { clearPersistedSidebarTabId } from "./utils/sidebarTabStorage";

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
	/**
	 * The active chat's children from the chat list cache, which watch
	 * events keep fresh. The entity cache's embedded children are only a
	 * fetch-time snapshot, so gating archive actions on them could leave
	 * the actions disabled after a child finishes. Undefined with the tree
	 * sidebar, where the entity is refetched on subagent events instead.
	 */
	activeChatChildren: readonly TypesGen.Chat[] | undefined;
	onRenameTitle?: (chatId: string, title: string) => Promise<void>;
	/** Opens the shared rename dialog so both menus drive the same instance. */
	onOpenRenameDialog?: (chat: TypesGen.Chat) => void;
	isSidebarCollapsed: boolean;
	onToggleSidebarCollapsed: () => void;
	onExpandSidebar: () => void;
	onChatReady: () => void;
}

const FILTER_MEMBERSHIP_EVENT_KINDS = new Set<TypesGen.ChatWatchEventKind>([
	"diff_status_change",
	"status_change",
]);

export const shouldInvalidateFilteredChatList = (
	chat: TypesGen.Chat,
	eventKind: TypesGen.ChatWatchEventKind,
): boolean =>
	chat.kind !== "subagent" && FILTER_MEMBERSHIP_EVENT_KINDS.has(eventKind);

// Summary and title generation can bill after the turn reports a non-active
// status, so invalidate the root-keyed cost query when those events arrive.
const POST_TURN_BILLED_EVENT_KINDS = new Set<TypesGen.ChatWatchEventKind>([
	"chat_summary_change",
	"summary_change",
	"title_change",
]);

export const chatCostIdToInvalidate = (
	chat: TypesGen.Chat,
	eventKind: TypesGen.ChatWatchEventKind,
): string | undefined => {
	if (POST_TURN_BILLED_EVENT_KINDS.has(eventKind)) {
		return getChatCostTreeID(chat);
	}
	if (eventKind !== "status_change" || isActiveChatStatus(chat.status)) {
		return undefined;
	}
	return getChatCostTreeID(chat);
};

export type PendingArchiveCascade = {
	readonly chatId: string;
	readonly title: string;
	readonly descendantCount: number;
};

/**
 * Describes the archive confirmation for a chat whose active tree rows
 * have descendants; undefined when the chat can be archived directly.
 * The count comes from cached active rows, not child_chat_count, which
 * also counts archived children.
 */
export const pendingArchiveCascadeFor = (
	chatId: string,
	title: string | undefined,
	descendantCount: number,
): PendingArchiveCascade | undefined =>
	descendantCount > 0
		? { chatId, title: title || "Untitled", descendantCount }
		: undefined;

export const archiveCascadeDescription = ({
	title,
	descendantCount,
}: PendingArchiveCascade): string =>
	`Archive "${title}" and ${descendantCount} ${
		descendantCount === 1 ? "chat" : "chats"
	} beneath it? Subagents are archived with them. Archiving fails if any of them is running, being interrupted, or waiting for approval.`;

const AgentsPageLayout: FC = () => {
	useAgentsPWA();
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const location = useLocation();
	const [searchParams, setSearchParams] = useSearchParams();
	const { agentId } = useParams();
	const { permissions, user } = useAuthenticated();
	const { organizations, experiments } = useDashboard();
	const isChatTreeEnabled = experiments.includes("chat-tree");
	const organizationName = getDefaultOrganizationName(organizations);
	const defaultOrganizationId = getDefaultOrganizationId(organizations);
	// The personal-overrides feature flag is deployment-wide but read through
	// an organization-scoped endpoint, so fall back to any accessible
	// organization for users outside the default organization.
	const personalOverridesOrganizationId =
		defaultOrganizationId || (organizations[0]?.id ?? "");
	const isAgentsAdmin = permissions.editDeploymentConfig;
	const canManageAgentSettings = canAccessCoderAgentsSettings(permissions);

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
	// With the tree on, the flat list only serves chats shared with the
	// viewer; owned chats come from the per organization tree queries.
	const wantsSharedChats = sidebarFilters.sources.includes("shared_with_me");
	const chatsQuery = useInfiniteQuery({
		...infiniteChats(
			isChatTreeEnabled
				? {
						archived: archivedFilter,
						chatStatus: chatStatusFilter,
						sources: ["shared_with_me"],
					}
				: {
						archived: archivedFilter,
						prStatuses: sidebarFilters.prStatuses,
						chatStatus: chatStatusFilter,
						sources: sidebarFilters.sources,
					},
		),
		enabled: !isChatTreeEnabled || wantsSharedChats,
	});
	// Empty when the experiment is off, so no tree request is ever made.
	const treeOrganizations = isChatTreeEnabled ? organizations : [];
	const chatTreeQueries = useQueries({
		queries: treeOrganizations.map((organization) =>
			chatTree(organization.id, { archived: archivedFilter }),
		),
	});
	const organizationModels = useOrganizationChatModels(
		organizations.map((organization) => organization.id),
	);
	const personalModelOverridesQuery = useQuery(
		userChatPersonalModelOverrides(personalOverridesOrganizationId),
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
			removeChatFromChatsByWorkspace(queryClient, chatId);
			clearChatErrorReason(chatId);
			clearPersistedSidebarTabId(chatId);
			clearPersistedRightPanelState(chatId);
			void invalidateChatListQueries(queryClient);
			void invalidateChatTreeQueries(queryClient);
			void invalidateChatEntity(queryClient, chatId);
			void invalidateChatsByWorkspace(queryClient);
			void invalidateChatSearches(queryClient);
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
		onError: (error, { chatId, workspaceId }) => {
			notifyArchiveAndDeleteFailed(
				queryClient.getQueryData<TypesGen.Workspace>(
					workspaceByIdKey(workspaceId),
				),
				error,
				(path) => navigate(path),
			);
			// The archive may have committed server-side even when the
			// request appeared to fail (transport errors), and on delete
			// failures the chat stays archived; refetch every chat
			// collection so all caches converge on the server.
			void invalidateChatListQueries(queryClient);
			void invalidateChatTreeQueries(queryClient);
			void invalidateChatEntity(queryClient, chatId);
			void invalidateChatsByWorkspace(queryClient);
			void invalidateChatSearches(queryClient);
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
	const sharedChatList = chatsQuery.data?.pages.flat() ?? [];
	const treeResponsesByOrganization = new Map<
		string,
		TypesGen.ChatTreeResponse
	>();
	for (const [index, organization] of treeOrganizations.entries()) {
		const response = chatTreeQueries[index]?.data;
		if (response) {
			treeResponsesByOrganization.set(organization.id, response);
		}
	}
	const treeChatList = [...treeResponsesByOrganization.values()].flatMap(
		(response) => response.chats,
	);
	// Tree responses are depth ordered and include the root row, so the
	// flat list handed to the sidebar drops the root and sorts by recency.
	const chatList = isChatTreeEnabled
		? [
				...treeChatList
					.filter((chat) => chat.kind !== "root")
					.sort((a, b) => b.updated_at.localeCompare(a.updated_at)),
				...sharedChatList,
			]
		: sharedChatList;
	const treeLoadError = chatTreeQueries.find((query) => query.error)?.error;
	const treeHasData = treeResponsesByOrganization.size > 0;
	const sidebarIsLoading = isChatTreeEnabled
		? chatTreeQueries.some((query) => query.isLoading) && !treeHasData
		: chatsQuery.isLoading;
	// A loaded tree is shown even when the shared list fails.
	const sidebarLoadError = isChatTreeEnabled
		? treeHasData
			? undefined
			: (treeLoadError ?? chatsQuery.error)
		: chatsQuery.error;
	const retrySidebarLoad = () => {
		if (isChatTreeEnabled) {
			for (const query of chatTreeQueries) {
				void query.refetch();
			}
		}
		if (!isChatTreeEnabled || wantsSharedChats) {
			void chatsQuery.refetch();
		}
	};
	const handleCreateChildChat = (parent: TypesGen.Chat) => {
		const state: NewChildChatLocationState = {
			parentChatId: parent.id,
			parentChatTitle: parent.title,
			parentOrganizationId: parent.organization_id,
		};
		navigate({ pathname: "/agents", search: location.search }, { state });
	};
	const treeData: ChatTreePanelData | undefined = isChatTreeEnabled
		? {
				organizations: treeOrganizations.map((organization) => ({
					id: organization.id,
					displayName: organization.display_name || organization.name,
				})),
				responsesByOrganization: treeResponsesByOrganization,
				sharedChats: sharedChatList,
				onCreateChildChat: handleCreateChildChat,
			}
		: undefined;
	const isArchiving =
		archiveAgentMutation.isPending || archiveAndDeleteMutation.isPending;
	const archivingChatId =
		(archiveAgentMutation.isPending
			? archiveAgentMutation.variables
			: undefined) ??
		(archiveAndDeleteMutation.isPending
			? archiveAndDeleteMutation.variables?.chatId
			: undefined);
	const [pendingArchiveCascade, setPendingArchiveCascade] =
		useState<PendingArchiveCascade | null>(null);
	const requestArchiveAgent = (chatId: string) => {
		if (isArchiving) {
			return;
		}
		if (isChatTreeEnabled) {
			const cascade = pendingArchiveCascadeFor(
				chatId,
				readChatFromSidebarCaches(queryClient, chatId)?.title,
				countChatTreeDescendantsInCaches(queryClient, chatId),
			);
			if (cascade) {
				setPendingArchiveCascade(cascade);
				return;
			}
		}
		archiveAgentMutation.mutate(chatId);
	};
	const handleConfirmArchiveCascade = () => {
		if (pendingArchiveCascade && !isArchiving) {
			archiveAgentMutation.mutate(pendingArchiveCascade.chatId);
		}
		setPendingArchiveCascade(null);
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
					? queryClient.getQueryData<TypesGen.Chat>(chatEntityKey(activeChatId))
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
				() => readChatFromSidebarCaches(queryClient, chatId)?.created_at,
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
	// The watch handler is subscribed once; it reads these through refs.
	const isChatTreeEnabledRef = useRef(isChatTreeEnabled);
	const currentUserIdRef = useRef(user.id);
	useEffect(() => {
		isChatTreeEnabledRef.current = isChatTreeEnabled;
		currentUserIdRef.current = user.id;
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
		updateChatTreeCaches(queryClient, (chats) => {
			let changed = false;
			const next = chats.map((c) => {
				if (c.id !== agentId || !c.has_unread) return c;
				changed = true;
				return { ...c, has_unread: false };
			});
			return changed ? next : chats;
		});
		void invalidateChatListQueries(queryClient);
		void invalidateChatTreeQueries(queryClient);
		void invalidateChatSearches(queryClient);
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
					const prevStatus = readChatFromSidebarCaches(
						queryClient,
						updatedChat.id,
					)?.status;
					// Only play the chime for top-level chats, not sub-agents.
					if (updatedChat.kind !== "subagent") {
						maybePlayChime(
							prevStatus,
							updatedChat.status,
							updatedChat.id,
							activeChatIDRef.current,
						);
					}

					if (chatEvent.kind === "deleted") {
						// The server publishes `deleted` when a chat is
						// archived (one event per family member); there is
						// no hard-delete wire event. Patch archive state in
						// place so an open route stays mounted and flips to
						// its read-only state.
						void cancelChatTreeRefetches(queryClient);
						applyWatchedChatArchived(queryClient, updatedChat);
						return;
					}
					if (chatEvent.kind === "diff_status_change") {
						// Only refetch the diff file contents. The chat's
						// diff_status field is already written into the
						// chatKey and infinite-list caches below.
						void invalidateChatDiffContents(queryClient, updatedChat.id);
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
					void cancelChatTreeRefetches(queryClient);
					void cancelLoadedChatEntityRefetch(queryClient, updatedChat.id);
					const treeEnabled = isChatTreeEnabledRef.current;
					// Tree rows never embed subagents; the toggled node's entity
					// query (and the open chat's) is refetched instead.
					if (
						treeEnabled &&
						updatedChat.kind === "subagent" &&
						updatedChat.parent_chat_id &&
						(chatEvent.kind === "created" || chatEvent.kind === "status_change")
					) {
						void invalidateChatEntity(queryClient, updatedChat.parent_chat_id);
					}

					if (chatEvent.kind === "created") {
						if (updatedChat.kind === "subagent" && updatedChat.parent_chat_id) {
							// Child chat: add to its parent's children
							// array. If the parent is not in any loaded
							// page, the child is silently dropped.
							addChildToParentInCache(
								queryClient,
								updatedChat,
								updatedChat.parent_chat_id,
							);
							// A family unarchive and a new sub-agent with a
							// mounted initial fetch both need entity recovery.
							const cachedChat = queryClient.getQueryData<TypesGen.Chat>(
								chatEntityKey(updatedChat.id),
							);
							if (
								cachedChat?.archived ||
								(cachedChat === undefined &&
									queryClient.getQueryState(chatEntityKey(updatedChat.id)) !==
										undefined)
							) {
								applyWatchedChatCreatedOrUnarchived(queryClient, updatedChat);
							}
						} else {
							// With the tree on, the flat list holds only shared chats,
							// so an owned chat is neither prepended into it nor a
							// reason to refetch it.
							const isSharedListChat =
								!treeEnabled ||
								updatedChat.owner_id !== currentUserIdRef.current;
							// `created` also fires for unarchive transitions.
							applyWatchedChatCreatedOrUnarchived(queryClient, updatedChat, {
								invalidateList: isSharedListChat,
							});
							if (isSharedListChat) {
								prependToInfiniteChatsCache(queryClient, updatedChat);
							}
						}
					} else {
						mergeWatchedChatIntoCaches(queryClient, updatedChat, {
							eventKind: chatEvent.kind,
							activeChatId: activeChatIDRef.current,
						});
						if (shouldInvalidateFilteredChatList(updatedChat, chatEvent.kind)) {
							void invalidateChatListQueries(queryClient);
						}
						if (shouldInvalidateChatSearches(chatEvent.kind)) {
							void invalidateChatSearches(queryClient);
						}
						if (shouldInvalidateChatsByWorkspace(chatEvent.kind)) {
							void invalidateChatsByWorkspace(queryClient);
						}
						const costChatId = chatCostIdToInvalidate(
							updatedChat,
							chatEvent.kind,
						);
						if (costChatId) {
							void invalidateChatCostTree(queryClient, costChatId);
						}
						if (chatEvent.kind === "context_dirty") {
							// The watch payload carries only the lightweight
							// context flags (the merge above applies them);
							// refetch the open chat to pull the pinned
							// resources the single-chat GET computes. Only the
							// active chat has an observer, so other chats are
							// merely marked stale.
							void invalidateChatEntity(queryClient, updatedChat.id);
						}
					}
				});
				return ws;
			},
			onOpen() {
				void invalidateChatListQueries(queryClient);
				void invalidateChatTreeQueries(queryClient);
				void invalidateChatsByWorkspace(queryClient);
				void invalidateChatSearches(queryClient);
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

	// The sidebar expects plain string error messages, but the outlet
	// context carries structured ChatDetailError objects.
	const sidebarChatErrorReasons = Object.fromEntries(
		Object.entries(chatErrorReasons).map(([chatId, error]) => [
			chatId,
			error.message,
		]),
	);

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
		activeChatChildren: isChatTreeEnabled
			? undefined
			: chatList.find((c) => c.id === agentId)?.children,
		onOpenRenameDialog: setChatPendingRename,
		isSidebarCollapsed,
		onToggleSidebarCollapsed: handleToggleSidebarCollapsed,
		onExpandSidebar: () => setIsSidebarCollapsed(false),
		onChatReady: () => {},
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
							: isSettingsDetail
								? "hidden sm:block shrink-0"
								: "order-2 sm:order-0 flex-1 min-h-0 border-b border-border-default sm:flex-none sm:border-t-0 sm:border-b-0",
						isSidebarCollapsed && "sm:hidden",
					)}
				>
					<ChatsSidebar
						chats={chatList}
						currentUserId={user.id}
						chatErrorReasons={sidebarChatErrorReasons}
						modelConfigs={organizationModels.models}
						isLoadingModelConfigs={organizationModels.isLoading}
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
						isLoading={sidebarIsLoading}
						loadError={sidebarLoadError}
						onRetryLoad={retrySidebarLoad}
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
						canManageAgentSettings={canManageAgentSettings}
						treeData={treeData}
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
			<ConfirmDialog
				type="delete"
				open={pendingArchiveCascade !== null}
				title="Archive chat"
				description={
					pendingArchiveCascade
						? archiveCascadeDescription(pendingArchiveCascade)
						: ""
				}
				confirmText="Archive"
				onClose={() => setPendingArchiveCascade(null)}
				onConfirm={handleConfirmArchiveCascade}
			/>
		</>
	);
};
export default AgentsPageLayout;
