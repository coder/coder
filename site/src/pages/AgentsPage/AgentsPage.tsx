import { API, watchChats } from "api/api";
import { getErrorMessage } from "api/errors";
import {
	archiveChat,
	chatDiffContentsKey,
	chatKey,
	chatModelConfigs,
	chatModels,
	createChat,
	infiniteChats,
	invalidateChatListQueries,
	prependToInfiniteChatsCache,
	readInfiniteChatsCache,
	unarchiveChat,
	updateInfiniteChatsCache,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { useAuthenticated } from "hooks";
import { useDashboard } from "modules/dashboard/useDashboard";
import {
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import {
	useInfiniteQuery,
	useMutation,
	useQuery,
	useQueryClient,
} from "react-query";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { createReconnectingWebSocket } from "utils/reconnectingWebSocket";
import {
	type CreateChatOptions,
	emptyInputStorageKey,
} from "./AgentCreateForm";
import { maybePlayChime } from "./AgentDetail/useAgentChime";
import type { AgentsOutletContext } from "./AgentsPageView";
import { AgentsPageView } from "./AgentsPageView";
import { getModelOptionsFromCatalog } from "./modelOptions";
import type { ChatDetailError } from "./usageLimitMessage";
import { useAgentsPageKeybindings } from "./useAgentsPageKeybindings";
import { useAgentsPWA } from "./useAgentsPWA";

const lastModelConfigIDStorageKey = "agents.last-model-config-id";
const nilUUID = "00000000-0000-0000-0000-000000000000";

// Type guard for SSE events from the chat list watch endpoint.
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
	const { permissions, user } = useAuthenticated();
	const { appearance } = useDashboard();
	const isAgentsAdmin =
		permissions.editDeploymentConfig ||
		user.roles.some((role) => role.name === "owner" || role.name === "admin");

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
	const chatModelsQuery = useQuery(chatModels());
	const chatModelConfigsQuery = useQuery(chatModelConfigs());
	const createMutation = useMutation(createChat(queryClient));
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
			await API.updateChat(chatId, { archived: true });
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
	const catalogModelOptions = useMemo(
		() =>
			getModelOptionsFromCatalog(
				chatModelsQuery.data,
				chatModelConfigsQuery.data,
			),
		[chatModelsQuery.data, chatModelConfigsQuery.data],
	);
	const modelConfigIDByModelID = useMemo(() => {
		const byModelID = new Map<string, string>();
		for (const config of chatModelConfigsQuery.data ?? []) {
			const provider = config.provider.trim().toLowerCase();
			const model = config.model.trim();
			if (!provider || !model) {
				continue;
			}
			const colonRef = `${provider}:${model}`;
			if (!byModelID.has(colonRef)) {
				byModelID.set(colonRef, config.id);
			}
			const slashRef = `${provider}/${model}`;
			if (!byModelID.has(slashRef)) {
				byModelID.set(slashRef, config.id);
			}
		}
		return byModelID;
	}, [chatModelConfigsQuery.data]);
	const setChatErrorReason = useCallback(
		(chatId: string, reason: ChatDetailError) => {
			const trimmedMessage = reason.message.trim();
			if (!chatId || !trimmedMessage) {
				return;
			}
			setChatErrorReasons((current) => {
				const existing = current[chatId];
				if (
					existing &&
					existing.kind === reason.kind &&
					existing.message === trimmedMessage
				) {
					return current;
				}
				return {
					...current,
					[chatId]: { kind: reason.kind, message: trimmedMessage },
				};
			});
		},
		[],
	);
	const clearChatErrorReason = useCallback((chatId: string) => {
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
	}, []);
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
	const requestArchiveAgent = useCallback(
		(chatId: string) => {
			if (!isArchiving) {
				archiveAgentMutation.mutate(chatId);
			}
		},
		[isArchiving, archiveAgentMutation],
	);
	const requestArchiveAndDeleteWorkspace = useCallback(
		(chatId: string, workspaceId: string) => {
			if (!isArchiving) {
				archiveAndDeleteMutation.mutate({ chatId, workspaceId });
			}
		},
		[isArchiving, archiveAndDeleteMutation],
	);
	const requestUnarchiveAgent = useCallback(
		(chatId: string) => {
			unarchiveAgentMutation.mutate(chatId);
		},
		[unarchiveAgentMutation],
	);
	const handleToggleSidebarCollapsed = useCallback(
		() => setIsSidebarCollapsed((prev) => !prev),
		[],
	);
	const outletContext: AgentsOutletContext = useMemo(
		() => ({
			chatErrorReasons,
			setChatErrorReason,
			clearChatErrorReason,
			requestArchiveAgent,
			requestUnarchiveAgent,
			requestArchiveAndDeleteWorkspace,
			isSidebarCollapsed,
			onToggleSidebarCollapsed: handleToggleSidebarCollapsed,
		}),
		[
			chatErrorReasons,
			setChatErrorReason,
			clearChatErrorReason,
			requestArchiveAgent,
			requestUnarchiveAgent,
			requestArchiveAndDeleteWorkspace,
			isSidebarCollapsed,
			handleToggleSidebarCollapsed,
		],
	);
	const handleCreateChat = async (options: CreateChatOptions) => {
		const { message, fileIDs, workspaceId, model } = options;
		const modelConfigID =
			(model && modelConfigIDByModelID.get(model)) || nilUUID;
		const content: TypesGen.ChatInputPart[] = [];
		if (message.trim()) {
			content.push({ type: "text", text: message });
		}
		if (fileIDs) {
			for (const fileID of fileIDs) {
				content.push({ type: "file", file_id: fileID });
			}
		}
		const createdChat = await createMutation.mutateAsync({
			content,
			workspace_id: workspaceId,
			model_config_id: modelConfigID,
		});

		if (typeof window !== "undefined") {
			if (modelConfigID !== nilUUID) {
				localStorage.setItem(lastModelConfigIDStorageKey, modelConfigID);
			} else {
				localStorage.removeItem(lastModelConfigIDStorageKey);
			}
		}

		navigate(`/agents/${createdChat.id}`);
	};

	const handleNewAgent = () => {
		// Only clear the draft when the user is already on the empty
		// state and explicitly requests a blank slate.  When navigating
		// back from a conversation the existing draft is preserved.
		if (typeof window !== "undefined" && !agentId) {
			localStorage.removeItem(emptyInputStorageKey);
		}
		navigate("/agents");
	};

	// Track the active chat ID in a ref so the watchChats
	// WebSocket handler can read it without re-subscribing on
	// every navigation.
	const activeChatIDRef = useRef(agentId);
	activeChatIDRef.current = agentId;

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

					// Read the previous status from the query cache, which
					// is synchronously updated by both the per-chat WebSocket
					// (via updateSidebarChat) and this handler. This avoids
					// the async-lag of a useEffect-based status map.
					const currentChats = readInfiniteChatsCache(queryClient);
					const prevStatus = currentChats?.find(
						(c) => c.id === updatedChat.id,
					)?.status; // Only play the chime for top-level chats, not sub-agents.
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
								didUpdate = true;
								return {
									...c,
									...(isStatusEvent && { status: updatedChat.status }),
									...(isTitleEvent && { title: updatedChat.title }),
									...(isDiffStatusEvent && {
										diff_status: updatedChat.diff_status,
									}),
									updated_at:
										c.updated_at > updatedChat.updated_at
											? c.updated_at
											: updatedChat.updated_at,
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
							return {
								...previousChat,
								...(isStatusEvent && { status: updatedChat.status }),
								...(isTitleEvent && { title: updatedChat.title }),
								...(isDiffStatusEvent && {
									diff_status: updatedChat.diff_status,
								}),
								updated_at:
									previousChat.updated_at > updatedChat.updated_at
										? previousChat.updated_at
										: updatedChat.updated_at,
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

	return (
		<AgentsPageView
			agentId={agentId}
			chatList={chatList}
			catalogModelOptions={catalogModelOptions}
			modelConfigs={chatModelConfigsQuery.data ?? []}
			logoUrl={appearance.logo_url}
			handleNewAgent={handleNewAgent}
			isCreating={createMutation.isPending}
			isArchiving={isArchiving}
			archivingChatId={archivingChatId}
			isChatsLoading={chatsQuery.isLoading}
			chatsLoadError={chatsQuery.error}
			onRetryChatsLoad={() => void chatsQuery.refetch()}
			onCollapseSidebar={() => setIsSidebarCollapsed(true)}
			isSidebarCollapsed={isSidebarCollapsed}
			onExpandSidebar={() => setIsSidebarCollapsed(false)}
			outletContext={outletContext}
			onCreateChat={handleCreateChat}
			createError={createMutation.error}
			modelCatalog={chatModelsQuery.data}
			isModelCatalogLoading={chatModelsQuery.isLoading}
			isModelConfigsLoading={chatModelConfigsQuery.isLoading}
			modelCatalogError={chatModelsQuery.error}
			isAgentsAdmin={isAgentsAdmin}
			hasNextPage={chatsQuery.hasNextPage}
			onLoadMore={() => void chatsQuery.fetchNextPage()}
			isFetchingNextPage={chatsQuery.isFetchingNextPage}
			archivedFilter={archivedFilter}
			onArchivedFilterChange={setArchivedFilter}
		/>
	);
};

export default AgentsPage;
