import { API, watchChat } from "api/api";
import {
	chat,
	chatDiffContentsKey,
	chatDiffStatus,
	chatDiffStatusKey,
	chatKey,
	chatModels,
	chats,
	chatsKey,
	createChatMessage,
	deleteChatQueuedMessage,
	interruptChat,
	promoteChatQueuedMessage,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { asRecord, asString } from "components/ai-elements/runtimeTypeUtils";
import { displayError } from "components/GlobalSnackbar/utils";
import { Skeleton } from "components/Skeleton/Skeleton";
import { getVSCodeHref, SESSION_TOKEN_PLACEHOLDER } from "modules/apps/apps";
import {
	type FC,
	startTransition,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useOutletContext, useParams } from "react-router";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { AgentChatInput } from "./AgentChatInput";
import type { AgentsOutletContext } from "./AgentsPage";
import { ConversationTimeline } from "./agentDetail/ConversationTimeline";
import {
	getLatestContextUsage,
	getParentChatID,
	getWorkspaceAgent,
	resolveModelFromChatConfig,
} from "./agentDetail/chatHelpers";
import {
	buildParsedMessageSections,
	buildSubagentTitles,
	parseMessagesWithMergedTools,
} from "./agentDetail/messageParsing";
import {
	applyMessagePartToStreamState,
	buildStreamTools,
} from "./agentDetail/streamState";
import { AgentDetailTopBarPortals } from "./agentDetail/TopBarPortals";
import type { StreamState } from "./agentDetail/types";
import { useMessageWindow } from "./agentDetail/useMessageWindow";
import {
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";
import { QueuedMessagesList } from "./QueuedMessagesList";

type CreateChatMessagePayload = TypesGen.CreateChatMessageRequest & {
	readonly model?: string;
};

const noopSetChatErrorReason: AgentsOutletContext["setChatErrorReason"] =
	() => {};
const noopClearChatErrorReason: AgentsOutletContext["clearChatErrorReason"] =
	() => {};
const noopSetRightPanelOpen: AgentsOutletContext["setRightPanelOpen"] =
	() => {};
const noopRequestArchiveAgent: AgentsOutletContext["requestArchiveAgent"] =
	() => {};

export const AgentDetail: FC = () => {
	const navigate = useNavigate();
	const { agentId } = useParams<{ agentId: string }>();
	const outletContext = useOutletContext<AgentsOutletContext | undefined>();
	const queryClient = useQueryClient();
	const [messagesById, setMessagesById] = useState<
		Map<number, TypesGen.ChatMessage>
	>(new Map());
	const [streamState, setStreamState] = useState<StreamState | null>(null);
	const [streamError, setStreamError] = useState<string | null>(null);
	const [queuedMessages, setQueuedMessages] = useState<
		readonly TypesGen.ChatQueuedMessage[]
	>([]);
	const [chatStatus, setChatStatus] = useState<TypesGen.ChatStatus | null>(
		null,
	);
	const [subagentStatusOverrides, setSubagentStatusOverrides] = useState<
		Map<string, TypesGen.ChatStatus>
	>(new Map());
	const [selectedModel, setSelectedModel] = useState("");
	const [showDiffPanel, setShowDiffPanel] = useState(false);
	const chatErrorReasons = outletContext?.chatErrorReasons ?? {};
	const setChatErrorReason =
		outletContext?.setChatErrorReason ?? noopSetChatErrorReason;
	const clearChatErrorReason =
		outletContext?.clearChatErrorReason ?? noopClearChatErrorReason;
	const setRightPanelOpen =
		outletContext?.setRightPanelOpen ?? noopSetRightPanelOpen;
	const requestArchiveAgent =
		outletContext?.requestArchiveAgent ?? noopRequestArchiveAgent;
	const streamResetFrameRef = useRef<number | null>(null);
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);

	const chatQuery = useQuery({
		...chat(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatsQuery = useQuery(chats());
	const workspaceId = chatQuery.data?.chat?.workspace_id;
	const workspaceAgentId = chatQuery.data?.chat?.workspace_agent_id;
	const workspaceQuery = useQuery({
		queryKey: ["workspace", "agent-detail", workspaceId ?? ""],
		queryFn: () => API.getWorkspace(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
	});
	const diffStatusQuery = useQuery({
		...chatDiffStatus(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatModelsQuery = useQuery(chatModels());
	const hasDiffStatus = Boolean(diffStatusQuery.data?.url);
	const workspace = workspaceQuery.data;
	const workspaceAgent = getWorkspaceAgent(workspace, workspaceAgentId);
	const chatData = chatQuery.data;
	const chatRecord = chatData?.chat;
	const chatMessages = chatData?.messages;
	const chatQueuedMessages = chatData?.queued_messages;
	const chatModelConfig = chatRecord?.model_config;

	useEffect(() => {
		if (hasDiffStatus) {
			setShowDiffPanel(true);
		}
	}, [hasDiffStatus]);

	useEffect(() => {
		setRightPanelOpen(hasDiffStatus && showDiffPanel);
		return () => {
			setRightPanelOpen(false);
		};
	}, [hasDiffStatus, setRightPanelOpen, showDiffPanel]);

	const modelOptions = useMemo(
		() => getModelOptionsFromCatalog(chatModelsQuery.data),
		[chatModelsQuery.data],
	);

	const sendMutation = useMutation(
		createChatMessage(queryClient, agentId ?? ""),
	);
	const interruptMutation = useMutation(
		interruptChat(queryClient, agentId ?? ""),
	);
	const deleteQueuedMutation = useMutation(
		deleteChatQueuedMessage(queryClient, agentId ?? ""),
	);
	const promoteQueuedMutation = useMutation(
		promoteChatQueuedMessage(queryClient, agentId ?? ""),
	);

	const updateSidebarChat = useCallback(
		(updater: (chat: TypesGen.Chat) => TypesGen.Chat) => {
			if (!agentId) {
				return;
			}

			queryClient.setQueryData<readonly TypesGen.Chat[] | undefined>(
				chatsKey,
				(currentChats) => {
					if (!currentChats) {
						return currentChats;
					}

					let didUpdate = false;
					const nextChats = currentChats.map((chat) => {
						if (chat.id !== agentId) {
							return chat;
						}
						didUpdate = true;
						return updater(chat);
					});

					return didUpdate ? nextChats : currentChats;
				},
			);
		},
		[agentId, queryClient],
	);

	const cancelScheduledStreamReset = useCallback(() => {
		if (streamResetFrameRef.current === null) {
			return;
		}
		window.cancelAnimationFrame(streamResetFrameRef.current);
		streamResetFrameRef.current = null;
	}, []);

	const scheduleStreamReset = useCallback(() => {
		cancelScheduledStreamReset();
		streamResetFrameRef.current = window.requestAnimationFrame(() => {
			setStreamState(null);
			streamResetFrameRef.current = null;
		});
	}, [cancelScheduledStreamReset]);

	useEffect(() => {
		if (!chatMessages) {
			setMessagesById(new Map());
			return;
		}
		setMessagesById(
			new Map(chatMessages.map((message) => [message.id, message])),
		);
	}, [chatMessages]);

	useEffect(() => {
		if (!chatRecord) {
			setChatStatus(null);
			return;
		}
		setChatStatus(chatRecord.status);
	}, [chatRecord]);

	useEffect(() => {
		if (!chatData) {
			setQueuedMessages([]);
			return;
		}
		setQueuedMessages(chatQueuedMessages ?? []);
	}, [chatData, chatQueuedMessages]);

	useEffect(() => {
		if (!chatModelConfig) {
			return;
		}
		setSelectedModel((current) => {
			if (current && modelOptions.some((model) => model.id === current)) {
				return current;
			}
			return resolveModelFromChatConfig(chatModelConfig, modelOptions);
		});
	}, [chatModelConfig, modelOptions]);

	useEffect(() => {
		if (!agentId) {
			return;
		}

		cancelScheduledStreamReset();
		setStreamState(null);
		setStreamError(null);
		setSubagentStatusOverrides(new Map());

		const socket = watchChat(agentId);
		const handleMessage = (
			payload: OneWayMessageEvent<TypesGen.ServerSentEvent>,
		) => {
			if (payload.parseError || !payload.parsedMessage) {
				setStreamError("Failed to parse chat stream update.");
				return;
			}
			if (payload.parsedMessage.type !== "data") {
				return;
			}

			const streamEvent = payload.parsedMessage
				.data as TypesGen.ChatStreamEvent & Record<string, unknown>;
			if (!streamEvent?.type) {
				return;
			}

			switch (streamEvent.type) {
				case "message": {
					const message = streamEvent.message;
					if (!message) {
						return;
					}
					setMessagesById((prev) => {
						const next = new Map(prev);
						next.set(message.id, message);
						return next;
					});
					scheduleStreamReset();
					updateSidebarChat((chat) => ({
						...chat,
						updated_at: message.created_at ?? new Date().toISOString(),
					}));
					void queryClient.invalidateQueries({ queryKey: chatsKey });
					return;
				}
				case "message_part": {
					const part = asRecord(streamEvent.message_part?.part);
					if (!part) {
						return;
					}
					cancelScheduledStreamReset();
					startTransition(() => {
						setStreamState((prev) => applyMessagePartToStreamState(prev, part));
					});
					return;
				}
				case "queue_update": {
					const queuedMsgs = streamEvent.queued_messages;
					setQueuedMessages(queuedMsgs ?? []);
					return;
				}
				case "status": {
					const status = asRecord(streamEvent.status);
					const nextStatus = asString(status?.status) as TypesGen.ChatStatus;
					if (!nextStatus) {
						return;
					}

					const eventChatID = asString(streamEvent.chat_id);
					if (eventChatID && eventChatID !== agentId) {
						setSubagentStatusOverrides((prev) => {
							if (prev.get(eventChatID) === nextStatus) {
								return prev;
							}
							const next = new Map(prev);
							next.set(eventChatID, nextStatus);
							return next;
						});
						return;
					}

					setChatStatus(nextStatus);
					if (agentId && nextStatus !== "error") {
						clearChatErrorReason(agentId);
					}
					updateSidebarChat((chat) => ({
						...chat,
						status: nextStatus,
						updated_at: new Date().toISOString(),
					}));
					if (agentId) {
						void Promise.all([
							queryClient.invalidateQueries({
								queryKey: chatDiffStatusKey(agentId),
							}),
							queryClient.invalidateQueries({
								queryKey: chatDiffContentsKey(agentId),
							}),
						]);
					}
					const shouldRefreshQueries =
						nextStatus === "completed" ||
						nextStatus === "error" ||
						nextStatus === "paused" ||
						nextStatus === "waiting";
					if (shouldRefreshQueries) {
						void Promise.all([
							queryClient.invalidateQueries({ queryKey: chatsKey }),
							queryClient.invalidateQueries({
								queryKey: chatKey(agentId),
							}),
						]);
					}
					return;
				}
				case "error": {
					const error = asRecord(streamEvent.error);
					const reason =
						asString(error?.message).trim() || "Chat processing failed.";
					setChatStatus("error");
					setStreamError(reason);
					if (agentId) {
						setChatErrorReason(agentId, reason);
					}
					updateSidebarChat((chat) => ({
						...chat,
						status: "error",
						updated_at: new Date().toISOString(),
					}));
					void Promise.all([
						queryClient.invalidateQueries({ queryKey: chatsKey }),
						queryClient.invalidateQueries({
							queryKey: chatKey(agentId),
						}),
					]);
					return;
				}
				default:
					break;
			}
		};

		const handleError = () => {
			setStreamError((current) => current ?? "Chat stream disconnected.");
			void Promise.all([
				queryClient.invalidateQueries({ queryKey: chatsKey }),
				queryClient.invalidateQueries({
					queryKey: chatKey(agentId),
				}),
			]);
		};

		socket.addEventListener("message", handleMessage);
		socket.addEventListener("error", handleError);

		return () => {
			socket.removeEventListener("message", handleMessage);
			socket.removeEventListener("error", handleError);
			socket.close();
			cancelScheduledStreamReset();
		};
	}, [
		agentId,
		cancelScheduledStreamReset,
		clearChatErrorReason,
		queryClient,
		scheduleStreamReset,
		setChatErrorReason,
		updateSidebarChat,
	]);

	const messages = useMemo(() => {
		const list = Array.from(messagesById.values());
		list.sort(
			(a, b) =>
				new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
		);
		return list;
	}, [messagesById]);
	const latestContextUsage = useMemo(
		() => getLatestContextUsage(messages),
		[messages],
	);
	const isStreaming =
		Boolean(streamState) ||
		chatStatus === "running" ||
		chatStatus === "pending";
	const hasModelOptions = modelOptions.length > 0;
	const hasConfiguredModels = hasConfiguredModelsInCatalog(
		chatModelsQuery.data,
	);
	const modelSelectorPlaceholder = getModelSelectorPlaceholder(
		modelOptions,
		chatModelsQuery.isLoading,
		hasConfiguredModels,
	);
	const modelCatalogStatusMessage = getModelCatalogStatusMessage(
		chatModelsQuery.data,
		modelOptions,
		chatModelsQuery.isLoading,
		Boolean(chatModelsQuery.error),
	);
	const inputStatusText = hasModelOptions
		? null
		: hasConfiguredModels
			? "Models are configured but unavailable. Ask an admin."
			: "No models configured. Ask an admin.";
	const isSubmissionPending =
		sendMutation.isPending || interruptMutation.isPending;
	const isInputDisabled = !hasModelOptions;

	const handleSend = async (message: string) => {
		if (
			!message.trim() ||
			isSubmissionPending ||
			!agentId ||
			!hasModelOptions
		) {
			return;
		}
		const request: CreateChatMessagePayload = {
			role: "user",
			content: JSON.parse(JSON.stringify(message)),
			model: selectedModel || undefined,
		};
		clearChatErrorReason(agentId);
		setStreamError(null);
		if (scrollContainerRef.current) {
			scrollContainerRef.current.scrollTop = 0;
		}
		await sendMutation.mutateAsync(request);
	};

	const handleInterrupt = () => {
		if (!agentId || interruptMutation.isPending) {
			return;
		}
		void interruptMutation.mutateAsync();
	};

	const streamTools = useMemo(
		() => buildStreamTools(streamState),
		[streamState],
	);
	const visibleMessages = messages.filter((message) => !message.hidden);
	const { hasMoreMessages, windowedMessages, loadMoreSentinelRef } =
		useMessageWindow({
			messages: visibleMessages,
			resetKey: agentId,
		});

	const parsedMessages = useMemo(
		() => parseMessagesWithMergedTools(windowedMessages),
		[windowedMessages],
	);
	const subagentTitles = useMemo(
		() => buildSubagentTitles(parsedMessages),
		[parsedMessages],
	);
	const parsedSections = useMemo(
		() => buildParsedMessageSections(parsedMessages),
		[parsedMessages],
	);
	const persistedErrorReason = agentId ? chatErrorReasons[agentId] : undefined;
	const detailErrorMessage =
		(chatStatus === "error" ? persistedErrorReason : undefined) || streamError;
	const isAwaitingFirstStreamChunk =
		!streamState && (chatStatus === "running" || chatStatus === "pending");
	const hasStreamOutput = Boolean(streamState) || isAwaitingFirstStreamChunk;

	const topBarTitleRef = outletContext?.topBarTitleRef;
	const topBarActionsRef = outletContext?.topBarActionsRef;
	const rightPanelRef = outletContext?.rightPanelRef;
	const chatTitle = chatQuery.data?.chat?.title;
	const parentChatID = getParentChatID(chatQuery.data?.chat);
	const parentChat = parentChatID
		? chatsQuery.data?.find((chat) => chat.id === parentChatID)
		: undefined;
	const workspaceRoute = workspace
		? `/@${workspace.owner_name}/${workspace.name}`
		: null;
	const canOpenWorkspace = Boolean(workspaceRoute);
	const canOpenEditors = Boolean(workspace && workspaceAgent);
	const shouldShowDiffPanel = hasDiffStatus && showDiffPanel;

	const handleOpenInEditor = async (editor: "cursor" | "vscode") => {
		if (!workspace || !workspaceAgent) {
			return;
		}

		try {
			const { key } = await API.getApiKey();
			const vscodeHref = getVSCodeHref("vscode", {
				owner: workspace.owner_name,
				workspace: workspace.name,
				token: key,
				agent: workspaceAgent.name,
				folder: workspaceAgent.expanded_directory,
			});

			if (editor === "cursor") {
				const cursorApp = workspaceAgent.apps.find((app) => {
					const name = (app.display_name ?? app.slug).toLowerCase();
					return app.slug.toLowerCase() === "cursor" || name === "cursor";
				});
				if (cursorApp?.external && cursorApp.url) {
					const href = cursorApp.url.includes(SESSION_TOKEN_PLACEHOLDER)
						? cursorApp.url.replaceAll(SESSION_TOKEN_PLACEHOLDER, key)
						: cursorApp.url;
					window.location.assign(href);
					return;
				}
				window.location.assign(vscodeHref.replace(/^vscode:/, "cursor:"));
				return;
			}

			window.location.assign(vscodeHref);
		} catch {
			displayError(
				editor === "cursor"
					? "Failed to open in Cursor."
					: "Failed to open in VS Code.",
			);
		}
	};

	const handleViewWorkspace = () => {
		if (!workspaceRoute) {
			return;
		}
		navigate(workspaceRoute);
	};

	const handleArchiveAgentAction = () => {
		if (!agentId) {
			return;
		}
		requestArchiveAgent(agentId);
	};

	if (chatQuery.isLoading) {
		return (
			<div className="mx-auto w-full max-w-3xl space-y-6 py-6">
				<div className="flex justify-end">
					<Skeleton className="h-10 w-2/3 rounded-xl" />
				</div>
				<div className="space-y-3">
					<Skeleton className="h-4 w-full" />
					<Skeleton className="h-4 w-5/6" />
					<Skeleton className="h-4 w-4/6" />
					<Skeleton className="h-4 w-full" />
					<Skeleton className="h-4 w-3/5" />
				</div>
			</div>
		);
	}

	if (!chatQuery.data || !agentId) {
		return (
			<div className="flex flex-1 items-center justify-center text-content-secondary">
				Chat not found
			</div>
		);
	}

	return (
		<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col">
			<AgentDetailTopBarPortals
				topBarTitleRef={topBarTitleRef}
				topBarActionsRef={topBarActionsRef}
				rightPanelRef={rightPanelRef}
				chatTitle={chatTitle}
				parentChat={parentChat}
				onOpenParentChat={(chatId) => navigate(`/agents/${chatId}`)}
				hasDiffStatus={hasDiffStatus}
				diffStatus={diffStatusQuery.data}
				showDiffPanel={showDiffPanel}
				onToggleDiffPanel={() => setShowDiffPanel((prev) => !prev)}
				canOpenEditors={canOpenEditors}
				canOpenWorkspace={canOpenWorkspace}
				onOpenInEditor={(editor) => {
					void handleOpenInEditor(editor);
				}}
				onViewWorkspace={handleViewWorkspace}
				onArchiveAgent={handleArchiveAgentAction}
				shouldShowDiffPanel={shouldShowDiffPanel}
				agentId={agentId}
			/>

			<div
				ref={scrollContainerRef}
				className="flex h-full flex-col-reverse overflow-y-auto [scrollbar-width:thin] [scrollbar-color:hsl(240_5%_26%)_transparent]"
			>
				<div>
					<ConversationTimeline
						isEmpty={visibleMessages.length === 0}
						hasMoreMessages={hasMoreMessages}
						loadMoreSentinelRef={loadMoreSentinelRef}
						parsedSections={parsedSections}
						hasStreamOutput={hasStreamOutput}
						streamState={streamState}
						streamTools={streamTools}
						subagentTitles={subagentTitles}
						subagentStatusOverrides={subagentStatusOverrides}
						isAwaitingFirstStreamChunk={isAwaitingFirstStreamChunk}
						detailErrorMessage={detailErrorMessage}
					/>

					{queuedMessages.length > 0 && (
						<QueuedMessagesList
							messages={queuedMessages}
							onDelete={(id) => deleteQueuedMutation.mutate(id)}
							onPromote={(id) => promoteQueuedMutation.mutate(id)}
						/>
					)}
					<AgentChatInput
						onSend={handleSend}
						isDisabled={isInputDisabled}
						isLoading={sendMutation.isPending}
						isStreaming={isStreaming}
						onInterrupt={handleInterrupt}
						isInterruptPending={interruptMutation.isPending}
						hasQueuedMessages={queuedMessages.length > 0}
						contextUsage={latestContextUsage}
						hasModelOptions={hasModelOptions}
						selectedModel={selectedModel}
						onModelChange={setSelectedModel}
						modelOptions={modelOptions}
						modelSelectorPlaceholder={modelSelectorPlaceholder}
						inputStatusText={inputStatusText}
						modelCatalogStatusMessage={modelCatalogStatusMessage}
						sticky
					/>
				</div>
			</div>
		</div>
	);
};

export default AgentDetail;
