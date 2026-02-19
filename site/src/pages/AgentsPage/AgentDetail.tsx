import { API, type ChatDiffStatusResponse, watchChat } from "api/api";
import {
	chat,
	chatDiffContentsKey,
	chatDiffStatus,
	chatDiffStatusKey,
	chats,
	chatModels,
	chatsKey,
	createChatMessage,
	interruptChat,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import {
	ConversationItem,
	Message,
	MessageContent,
	type ModelSelectorOption,
	Response,
	Shimmer,
	Thinking,
	Tool,
} from "components/ai-elements";
import { Button } from "components/Button/Button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from "components/DropdownMenu/DropdownMenu";
import { displayError } from "components/GlobalSnackbar/utils";
import { Skeleton } from "components/Skeleton/Skeleton";
import {
	ArchiveIcon,
	ChevronRightIcon,
	EllipsisIcon,
	ExternalLinkIcon,
	MonitorIcon,
	PanelRightCloseIcon,
	PanelRightOpenIcon,
} from "lucide-react";
import { SESSION_TOKEN_PLACEHOLDER, getVSCodeHref } from "modules/apps/apps";
import {
	type FC,
	memo,
	startTransition,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useOutletContext, useParams } from "react-router";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import { AgentChatInput } from "./AgentChatInput";
import type { AgentsOutletContext } from "./AgentsPage";
import { FilesChangedPanel } from "./FilesChangedPanel";
import {
	getModelCatalogStatusMessage,
	getModelOptionsFromCatalog,
	getModelSelectorPlaceholder,
	hasConfiguredModelsInCatalog,
} from "./modelOptions";

type ChatModelOption = ModelSelectorOption;

const asRecord = (value: unknown): Record<string, unknown> | null => {
	if (!value || typeof value !== "object" || Array.isArray(value)) {
		return null;
	}
	return value as Record<string, unknown>;
};

const asString = (value: unknown): string => {
	return typeof value === "string" ? value : "";
};

const asNonEmptyString = (value: unknown): string | undefined => {
	const next = asString(value).trim();
	return next.length > 0 ? next : undefined;
};

type ChatWithHierarchyMetadata = TypesGen.Chat & {
	readonly parent_chat_id?: string;
};

const getParentChatID = (chat: TypesGen.Chat | undefined): string | undefined => {
	return asNonEmptyString(
		(chat as ChatWithHierarchyMetadata | undefined)?.parent_chat_id,
	);
};

const appendText = (current: string, next: string): string => {
	const trimmed = next.trim();
	if (!trimmed) {
		return current;
	}
	if (!current) {
		return next;
	}
	return `${current}\n${next}`;
};

const normalizeBlockType = (value: unknown): string =>
	asString(value).toLowerCase().replace(/_/g, "-");

const mergeStreamPayload = (
	existing: unknown,
	value: unknown,
	delta: unknown,
): unknown => {
	if (value !== undefined) {
		return value;
	}

	const chunk = typeof delta === "string" ? delta : "";
	if (!chunk) {
		return existing;
	}

	if (typeof existing === "string") {
		return `${existing}${chunk}`;
	}

	if (existing === undefined) {
		return chunk;
	}

	return existing;
};

type ParsedToolCall = {
	id: string;
	name: string;
	args?: unknown;
};

type ParsedToolResult = {
	id: string;
	name: string;
	result?: unknown;
	isError: boolean;
};

type MergedTool = {
	id: string;
	name: string;
	args?: unknown;
	result?: unknown;
	isError: boolean;
	status: "completed" | "error" | "running";
};

type ParsedMessageContent = {
	markdown: string;
	reasoning: string;
	toolCalls: ParsedToolCall[];
	toolResults: ParsedToolResult[];
	tools: MergedTool[];
};

const emptyParsedMessageContent = (): ParsedMessageContent => ({
	markdown: "",
	reasoning: "",
	toolCalls: [],
	toolResults: [],
	tools: [],
});

const mergeTools = (
	calls: ParsedToolCall[],
	results: ParsedToolResult[],
): MergedTool[] => {
	const resultById = new Map(results.map((r) => [r.id, r]));
	const seen = new Set<string>();
	const merged: MergedTool[] = [];

	for (const call of calls) {
		seen.add(call.id);
		const result = resultById.get(call.id);
		merged.push({
			id: call.id,
			name: call.name,
			args: call.args,
			result: result?.result,
			isError: result?.isError ?? false,
			status: result ? (result.isError ? "error" : "completed") : "completed",
		});
	}

	// Results without a matching call (standalone tool-result parts).
	for (const result of results) {
		if (!seen.has(result.id)) {
			merged.push({
				id: result.id,
				name: result.name,
				result: result.result,
				isError: result.isError,
				status: result.isError ? "error" : "completed",
			});
		}
	}

	return merged;
};

const parseMessageContent = (content: unknown): ParsedMessageContent => {
	if (typeof content === "string") {
		return {
			...emptyParsedMessageContent(),
			markdown: content,
		};
	}

	if (Array.isArray(content)) {
		const parsed = emptyParsedMessageContent();
		for (const [index, block] of content.entries()) {
			if (typeof block === "string") {
				parsed.markdown = appendText(parsed.markdown, block);
				continue;
			}

			const typedBlock = asRecord(block);
			if (!typedBlock) {
				continue;
			}

			switch (normalizeBlockType(typedBlock.type)) {
				case "text":
					parsed.markdown = appendText(
						parsed.markdown,
						asString(typedBlock.text),
					);
					break;
				case "reasoning":
				case "thinking":
					parsed.reasoning = appendText(
						parsed.reasoning,
						asString(typedBlock.text),
					);
					break;
				case "tool-call":
				case "toolcall": {
					const name =
						asString(typedBlock.tool_name) || asString(typedBlock.name);
					const id =
						asString(typedBlock.tool_call_id) ||
						asString(typedBlock.id) ||
						`tool-call-${index}`;
					parsed.toolCalls.push({
						id,
						name: name || "Tool",
						args: typedBlock.args ?? typedBlock.input ?? typedBlock.arguments,
					});
					break;
				}
				case "tool-result":
				case "toolresult": {
					const name =
						asString(typedBlock.tool_name) || asString(typedBlock.name);
					const id =
						asString(typedBlock.tool_call_id) ||
						asString(typedBlock.id) ||
						`tool-result-${index}`;
					parsed.toolResults.push({
						id,
						name: name || "Tool",
						result:
							typedBlock.result ??
							typedBlock.output ??
							typedBlock.content ??
							typedBlock.data,
						isError: Boolean(typedBlock.is_error ?? typedBlock.error),
					});
					break;
				}
				default:
					parsed.markdown = appendText(
						parsed.markdown,
						asString(typedBlock.text),
					);
					break;
			}
		}
		return parsed;
	}

	if (content === null || content === undefined) {
		return emptyParsedMessageContent();
	}

	const typedContent = asRecord(content);
	if (!typedContent) {
		return {
			...emptyParsedMessageContent(),
			markdown: String(content),
		};
	}

	if (Array.isArray(typedContent.parts)) {
		const parsed = emptyParsedMessageContent();
		for (const part of typedContent.parts) {
			const typedPart = asRecord(part);
			if (!typedPart) {
				continue;
			}
			if (normalizeBlockType(typedPart.type) === "text") {
				parsed.markdown = appendText(parsed.markdown, asString(typedPart.text));
			}
		}
		return parsed;
	}

	if (typedContent.type) {
		return parseMessageContent([typedContent]);
	}

	return {
		...emptyParsedMessageContent(),
		markdown: asString(typedContent.text) || asString(typedContent.content),
	};
};

const resolveModelFromChatConfig = (
	modelConfig: TypesGen.Chat["model_config"],
	modelOptions: readonly ChatModelOption[],
): string => {
	if (modelOptions.length === 0) {
		return "";
	}

	if (!modelConfig || typeof modelConfig !== "object") {
		return modelOptions[0]?.id ?? "";
	}

	const typedModelConfig = modelConfig as Record<string, unknown>;
	const model = asString(typedModelConfig.model);
	const provider = asString(typedModelConfig.provider);

	const candidates = [model];
	if (provider && model) {
		candidates.push(`${provider}:${model}`);
	}

	for (const candidate of candidates) {
		const match = modelOptions.find((option) => option.id === candidate);
		if (match) {
			return match.id;
		}
	}

	if (model) {
		const modelMatch = modelOptions.find(
			(option) =>
				option.model === model && (!provider || option.provider === provider),
		);
		if (modelMatch) {
			return modelMatch.id;
		}
	}

	return modelOptions[0]?.id ?? "";
};

type StreamToolCall = {
	id: string;
	name: string;
	args?: unknown;
};

type StreamToolResult = {
	id: string;
	name: string;
	result?: unknown;
	isError: boolean;
};

type StreamState = {
	content: string;
	reasoning: string;
	toolCalls: Record<string, StreamToolCall>;
	toolResults: Record<string, StreamToolResult>;
};

const createEmptyStreamState = (): StreamState => ({
	content: "",
	reasoning: "",
	toolCalls: {},
	toolResults: {},
});

/**
 * Collects all tool results across every message into a single
 * lookup map keyed by tool call ID. This lets us match a tool
 * call in one assistant message with its result that arrives in
 * a later message.
 */
const buildGlobalToolResultMap = (
	messages: TypesGen.ChatMessage[],
): Map<string, ParsedToolResult> => {
	const map = new Map<string, ParsedToolResult>();
	for (const message of messages) {
		const parsed =
			Array.isArray(message.parts) && message.parts.length > 0
				? parseMessageContent(message.parts)
				: parseMessageContent(message.content);
		for (const result of parsed.toolResults) {
			map.set(result.id, result);
		}
	}
	return map;
};

const parseChatMessageContent = (
	message: TypesGen.ChatMessage,
	globalToolResults: Map<string, ParsedToolResult>,
): ParsedMessageContent => {
	const parsed =
		Array.isArray(message.parts) && message.parts.length > 0
			? parseMessageContent(message.parts)
			: parseMessageContent(message.content);

	// Merge using the global result map so tool calls find their
	// results even when they arrive in a separate message.
	const resultById = new Map<string, ParsedToolResult>();
	for (const r of parsed.toolResults) {
		resultById.set(r.id, r);
	}
	for (const call of parsed.toolCalls) {
		if (!resultById.has(call.id)) {
			const global = globalToolResults.get(call.id);
			if (global) {
				resultById.set(global.id, global);
			}
		}
	}
	parsed.tools = mergeTools(parsed.toolCalls, Array.from(resultById.values()));
	return parsed;
};

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

interface DiffStatsBadgeProps {
	status: ChatDiffStatusResponse;
	isOpen: boolean;
	onToggle: () => void;
}

const DiffStatsBadge: FC<DiffStatsBadgeProps> = ({
	status,
	isOpen,
	onToggle,
}) => {
	const additions = status.additions ?? 0;
	const deletions = status.deletions ?? 0;

	return (
		<div
			role="button"
			tabIndex={0}
			onClick={onToggle}
			onKeyDown={(e) => {
				if (e.key === "Enter" || e.key === " ") {
					onToggle();
				}
			}}
			className="flex cursor-pointer items-center gap-3 px-2 py-1 text-content-secondary transition-colors hover:text-content-primary"
		>
			<span className="font-mono text-sm font-semibold text-content-success">
				+{additions}
			</span>
			<span className="font-mono text-sm font-semibold text-content-destructive">
				−{deletions}
			</span>
			{isOpen ? (
				<PanelRightCloseIcon className="h-4 w-4" />
			) : (
				<PanelRightOpenIcon className="h-4 w-4" />
			)}
		</div>
	);
};

// ---------------------------------------------------------------------------
// Memoized sub-components
// ---------------------------------------------------------------------------

/**
 * Renders a single historic chat message. Wrapped in React.memo so
 * it only re-renders when its own parsed content changes — not on
 * every stream chunk or input keystroke.
 */
const ChatMessageItem = memo<{
	message: TypesGen.ChatMessage;
	parsed: ParsedMessageContent;
}>(({ message, parsed }) => {
	const isUser = message.role === "user";

	// Skip messages that only carry tool results. Those results
	// are shown inline with the tool-call message they belong to.
	if (
		parsed.toolResults.length > 0 &&
		parsed.toolCalls.length === 0 &&
		parsed.markdown === "" &&
		parsed.reasoning === ""
	) {
		return null;
	}

	const hasRenderableContent =
		parsed.markdown !== "" ||
		parsed.reasoning !== "" ||
		parsed.tools.length > 0;
	const conversationItemProps = {
		role: (isUser ? "user" as const : "assistant" as const),
	};

	return (
		<ConversationItem {...conversationItemProps}>
			{isUser ? (
				<Message className="my-2 w-full max-w-none">
					<MessageContent className="rounded-lg border border-solid border-border-default bg-surface-secondary px-3 py-2 font-sans shadow-sm">
						{parsed.markdown || ""}
					</MessageContent>
				</Message>
			) : (
				<Message className="w-full">
					<MessageContent className="whitespace-normal">
						<div className="space-y-3">
							{parsed.markdown && <Response>{parsed.markdown}</Response>}
							{parsed.reasoning && <Thinking>{parsed.reasoning}</Thinking>}
							{parsed.tools.map((tool) => (
								<Tool
									key={tool.id}
									name={tool.name}
									args={tool.args}
									result={tool.result}
									status={tool.status}
									isError={tool.isError}
								/>
							))}
							{!hasRenderableContent && (
								<div className="text-xs text-content-secondary">
									Message has no renderable content.
								</div>
							)}
						</div>
					</MessageContent>
				</Message>
			)}
		</ConversationItem>
	);
});
ChatMessageItem.displayName = "ChatMessageItem";

/**
 * Renders the live streaming assistant output. Isolated via memo so
 * historic messages and the input area are not re-rendered on each
 * chunk.
 */
const StreamingOutput = memo<{
	streamState: StreamState | null;
	streamTools: MergedTool[];
}>(({ streamState, streamTools }) => {
	const conversationItemProps = { role: "assistant" as const };

	return (
		<ConversationItem {...conversationItemProps}>
			<Message>
				<MessageContent className="whitespace-normal">
					<div className="space-y-3">
						{streamState?.content ? (
							<Response>{streamState.content}</Response>
						) : (
							<Shimmer as="span" className="text-sm">
								Thinking...
							</Shimmer>
						)}
						{streamState?.reasoning && (
							<Thinking>{streamState.reasoning}</Thinking>
						)}
						{streamTools.map((tool) => (
							<Tool
								key={tool.id}
								name={tool.name}
								args={tool.args}
								result={tool.result}
								status={tool.status}
								isError={tool.isError}
							/>
						))}
					</div>
				</MessageContent>
			</Message>
		</ConversationItem>
	);
});
StreamingOutput.displayName = "StreamingOutput";

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

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
	const [chatStatus, setChatStatus] = useState<TypesGen.ChatStatus | null>(
		null,
	);
	const [selectedModel, setSelectedModel] = useState("");
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

	const chatQuery = useQuery({
		...chat(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatsQuery = useQuery(chats());
	const workspaceId = chatQuery.data?.chat.workspace_id;
	const workspaceAgentId = chatQuery.data?.chat.workspace_agent_id;
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
	const [showDiffPanel, setShowDiffPanel] = useState(false);

	const workspaceAgent = useMemo(() => {
		const workspace = workspaceQuery.data;
		if (!workspace) {
			return undefined;
		}
		const agents = workspace.latest_build.resources.flatMap(
			(resource) => resource.agents ?? [],
		);
		if (agents.length === 0) {
			return undefined;
		}
		return (
			agents.find((agent) => agent.id === workspaceAgentId) ??
			agents[0]
		);
	}, [workspaceAgentId, workspaceQuery.data]);

	// Auto-open the diff panel when diff status becomes available.
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

	const catalogModelOptions = useMemo(
		() => getModelOptionsFromCatalog(chatModelsQuery.data),
		[chatModelsQuery.data],
	);
	const modelOptions = catalogModelOptions;

	const sendMutation = useMutation(
		createChatMessage(queryClient, agentId ?? ""),
	);
	const interruptMutation = useMutation(
		interruptChat(queryClient, agentId ?? ""),
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
		if (!chatQuery.data) {
			return;
		}
		setMessagesById(
			new Map(chatQuery.data.messages.map((message) => [message.id, message])),
		);
		setChatStatus(chatQuery.data.chat.status);
	}, [chatQuery.data]);

	useEffect(() => {
		if (!chatQuery.data) {
			return;
		}
		setSelectedModel((current) => {
			if (current && modelOptions.some((model) => model.id === current)) {
				return current;
			}
			return resolveModelFromChatConfig(
				chatQuery.data.chat.model_config,
				modelOptions,
			);
		});
	}, [chatQuery.data, modelOptions]);

	useEffect(() => {
		if (!agentId) {
			return;
		}

		cancelScheduledStreamReset();
		setStreamState(null);
		setStreamError(null);

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
					const messagePart = streamEvent.message_part;
					const part = messagePart?.part;
					if (!part) {
						return;
					}
					cancelScheduledStreamReset();

					// Wrap stream state updates in startTransition so React
					// treats them as low-priority — keeping user interactions
					// (typing, clicking) responsive during rapid streaming.
					switch (normalizeBlockType(part.type)) {
						case "text":
							startTransition(() => {
								setStreamState((prev) => {
									const nextState: StreamState =
										prev ?? createEmptyStreamState();
									return {
										...nextState,
										content: `${nextState.content}${asString(part.text)}`,
									};
								});
							});
							return;
						case "reasoning":
						case "thinking":
							startTransition(() => {
								setStreamState((prev) => {
									const nextState: StreamState =
										prev ?? createEmptyStreamState();
									return {
										...nextState,
										reasoning: `${nextState.reasoning}${asString(part.text)}`,
									};
								});
							});
							return;
						case "tool-call":
						case "toolcall": {
							const toolName = asString(part.tool_name);

							startTransition(() => {
								setStreamState((prev) => {
									const nextState: StreamState =
										prev ?? createEmptyStreamState();
									const existingByName = Object.values(
										nextState.toolCalls,
									).find((call) => call.name === toolName);
									const toolCallID =
										asString(part.tool_call_id) ||
										existingByName?.id ||
										`tool-call-${Object.keys(nextState.toolCalls).length + 1}`;
									const existing = nextState.toolCalls[toolCallID];
									const nextArgs = mergeStreamPayload(
										existing?.args,
										part.args,
										part.args_delta,
									);

									return {
										...nextState,
										toolCalls: {
											...nextState.toolCalls,
											[toolCallID]: {
												id: toolCallID,
												name: toolName || existing?.name || "Tool",
												args: nextArgs,
											},
										},
									};
								});
							});
							return;
						}
						case "tool-result":
						case "toolresult": {
							const toolName = asString(part.tool_name);

							startTransition(() => {
								setStreamState((prev) => {
									const nextState: StreamState =
										prev ?? createEmptyStreamState();
									const existingByName = Object.values(
										nextState.toolResults,
									).find((result) => result.name === toolName);
									const existingCallByName = Object.values(
										nextState.toolCalls,
									).find((call) => call.name === toolName);
									const toolCallID =
										asString(part.tool_call_id) ||
										existingByName?.id ||
										existingCallByName?.id ||
										`tool-result-${Object.keys(nextState.toolResults).length + 1}`;
									const existing = nextState.toolResults[toolCallID];
									const nextResult = mergeStreamPayload(
										existing?.result,
										part.result,
										part.result_delta,
									);

									return {
										...nextState,
										toolResults: {
											...nextState.toolResults,
											[toolCallID]: {
												id: toolCallID,
												name: toolName || existing?.name || "Tool",
												result: nextResult,
												isError: existing?.isError || Boolean(part.is_error),
											},
										},
									};
								});
							});
							return;
						}
						default:
							return;
					}
				}
				case "status": {
					const status = asRecord(streamEvent.status);
					const nextStatus = asString(status?.status) as TypesGen.ChatStatus;
					if (nextStatus) {
						setChatStatus(nextStatus);
						if (agentId && nextStatus !== "error") {
							clearChatErrorReason(agentId);
						}
						updateSidebarChat((chat) => ({
							...chat,
							status: nextStatus,
							updated_at: new Date().toISOString(),
						}));
						// Always refresh diff queries on any status event
						// because the background refresh may have discovered
						// a new PR or updated diff contents.
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
							void queryClient.invalidateQueries({ queryKey: chatsKey });
						}
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
					void queryClient.invalidateQueries({ queryKey: chatsKey });
					return;
				}
				default:
					break;
			}
		};

		const handleError = () => {
			setStreamError((current) => current ?? "Chat stream disconnected.");
			void queryClient.invalidateQueries({ queryKey: chatsKey });
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
	const isInputDisabled = isSubmissionPending || !hasModelOptions;

	// Stable callback refs — the actual implementation is updated on
	// every render, but the reference passed to ChatInput never changes.
	// This prevents ChatInput from re-rendering when unrelated parent
	// state (streamState, messagesById) changes.
	const handleSendRef = useRef<(message: string) => Promise<void>>(
		async () => {},
	);
	handleSendRef.current = async (message: string) => {
		if (
			!message.trim() ||
			isSubmissionPending ||
			!agentId ||
			!hasModelOptions
		) {
			return;
		}
		if (isStreaming) {
			await interruptMutation.mutateAsync();
		}
		const request: CreateChatMessagePayload = {
			role: "user",
			content: JSON.parse(JSON.stringify(message)),
			model: selectedModel || undefined,
		};
		clearChatErrorReason(agentId);
		setStreamError(null);
		await sendMutation.mutateAsync(request);
	};

	const handleInterruptRef = useRef<() => void>(() => {});
	handleInterruptRef.current = () => {
		if (!agentId || interruptMutation.isPending) {
			return;
		}
		void interruptMutation.mutateAsync();
	};

	// Stable wrappers that never change identity.
	const stableOnSend = useCallback(
		(message: string) => handleSendRef.current(message),
		[],
	);
	const stableOnInterrupt = useCallback(() => handleInterruptRef.current(), []);

	const streamTools = useMemo((): MergedTool[] => {
		if (!streamState) {
			return [];
		}
		const calls = Object.values(streamState.toolCalls);
		const seen = new Set<string>();
		const merged: MergedTool[] = [];

		for (const call of calls) {
			seen.add(call.id);
			const result = streamState.toolResults[call.id];
			merged.push({
				id: call.id,
				name: call.name,
				args: call.args,
				result: result?.result,
				isError: result?.isError ?? false,
				status: result ? (result.isError ? "error" : "completed") : "running",
			});
		}

		for (const result of Object.values(streamState.toolResults)) {
			if (!seen.has(result.id)) {
				merged.push({
					id: result.id,
					name: result.name,
					result: result.result,
					isError: result.isError,
					status: result.isError ? "error" : "completed",
				});
			}
		}

		return merged;
	}, [streamState]);

	const visibleMessages = useMemo(
		() => messages.filter((message) => !message.hidden),
		[messages],
	);
	const globalToolResults = useMemo(
		() => buildGlobalToolResultMap(visibleMessages),
		[visibleMessages],
	);

	// Pre-compute parsed content for all visible messages. This runs
	// only when visibleMessages changes (at message boundaries), not
	// on every stream chunk. Each entry keeps a stable reference so
	// ChatMessageItem can bail out via React.memo.
	const parsedMessages = useMemo(
		() =>
			visibleMessages.map((message) => ({
				message,
				parsed: parseChatMessageContent(message, globalToolResults),
			})),
		[visibleMessages, globalToolResults],
	);

	// Group messages into sections so each user message can be CSS
	// sticky within its section. When the section scrolls out, the
	// sticky user message scrolls away and the next one takes over
	// — no JavaScript scroll tracking needed.
	const messageSections = useMemo(() => {
		const sections: Array<{
			userEntry: (typeof parsedMessages)[number] | null;
			entries: typeof parsedMessages;
		}> = [];

		for (const entry of parsedMessages) {
			if (entry.message.role === "user") {
				sections.push({ userEntry: entry, entries: [entry] });
			} else if (sections.length === 0) {
				sections.push({ userEntry: null, entries: [entry] });
			} else {
				sections[sections.length - 1].entries.push(entry);
			}
		}

		return sections;
	}, [parsedMessages]);

	const persistedErrorReason = agentId ? chatErrorReasons[agentId] : undefined;
	const detailErrorMessage =
		(chatStatus === "error" ? persistedErrorReason : undefined) || streamError;
	const hasStreamOutput =
		chatStatus === "running" ||
		chatStatus === "pending" ||
		(!!streamState &&
			(streamState.content !== "" ||
				streamState.reasoning !== "" ||
				streamTools.length > 0));

	const topBarTitleRef = outletContext?.topBarTitleRef;
	const topBarActionsRef = outletContext?.topBarActionsRef;
	const rightPanelRef = outletContext?.rightPanelRef;
	const chatTitle = chatQuery.data?.chat.title;
	const parentChatID = getParentChatID(chatQuery.data?.chat);
	const parentChat = parentChatID
		? chatsQuery.data?.find((chat) => chat.id === parentChatID)
		: undefined;
	const workspace = workspaceQuery.data;
	const workspaceRoute = workspace
		? `/@${workspace.owner_name}/${workspace.name}`
		: null;
	const canOpenWorkspace = Boolean(workspaceRoute);
	const canOpenEditors = Boolean(workspace && workspaceAgent);
	const shouldShowDiffPanel = hasDiffStatus && showDiffPanel;

	const handleOpenInEditor = useCallback(
		async (editor: "cursor" | "vscode") => {
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
		},
		[workspace, workspaceAgent],
	);

	const handleViewWorkspace = useCallback(() => {
		if (!workspaceRoute) {
			return;
		}
		navigate(workspaceRoute);
	}, [navigate, workspaceRoute]);

	const handleArchiveAgentAction = useCallback(() => {
		if (!agentId) {
			return;
		}
		requestArchiveAgent(agentId);
	}, [agentId, requestArchiveAgent]);

	if (chatQuery.isLoading) {
		return (
			<div className="mx-auto w-full max-w-3xl space-y-6 py-6">
				{/* User message skeleton */}
				<div className="flex justify-end">
					<Skeleton className="h-10 w-2/3 rounded-xl" />
				</div>
				{/* Assistant response skeleton */}
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

	const chatContent = (
		<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col">
			{chatTitle &&
				topBarTitleRef?.current &&
				createPortal(
					<div className="flex min-w-0 items-center gap-1.5">
						{parentChat && (
							<>
								<Button
									size="sm"
									variant="subtle"
									className="h-auto max-w-[16rem] rounded-sm px-1 py-0.5 text-xs text-content-secondary shadow-none hover:bg-transparent hover:text-content-primary"
									onClick={() => navigate(`/agents/${parentChat.id}`)}
								>
									<span className="truncate">{parentChat.title}</span>
								</Button>
								<ChevronRightIcon className="h-3.5 w-3.5 shrink-0 text-content-secondary/70" />
							</>
						)}
						<span className="truncate text-sm text-content-primary">
							{chatTitle}
						</span>
					</div>,
					topBarTitleRef.current,
				)}
			{hasDiffStatus &&
				diffStatusQuery.data &&
				topBarActionsRef?.current &&
				createPortal(
					<DiffStatsBadge
						status={diffStatusQuery.data}
						isOpen={showDiffPanel}
						onToggle={() => setShowDiffPanel((prev) => !prev)}
					/>,
					topBarActionsRef.current,
				)}
			{topBarActionsRef?.current &&
				createPortal(
					<DropdownMenu>
						<DropdownMenuTrigger asChild>
							<Button
								size="icon"
								variant="subtle"
								className="h-7 w-7 text-content-secondary hover:text-content-primary"
								aria-label="Open agent actions"
							>
								<EllipsisIcon className="h-4 w-4" />
							</Button>
						</DropdownMenuTrigger>
						<DropdownMenuContent align="end">
							<DropdownMenuItem
								disabled={!canOpenEditors}
								onSelect={() => {
									void handleOpenInEditor("cursor");
								}}
							>
								<ExternalLinkIcon className="h-3.5 w-3.5" />
								Open in Cursor
							</DropdownMenuItem>
							<DropdownMenuItem
								disabled={!canOpenEditors}
								onSelect={() => {
									void handleOpenInEditor("vscode");
								}}
							>
								<ExternalLinkIcon className="h-3.5 w-3.5" />
								Open in VS Code
							</DropdownMenuItem>
							<DropdownMenuItem
								disabled={!canOpenWorkspace}
								onSelect={handleViewWorkspace}
							>
								<MonitorIcon className="h-3.5 w-3.5" />
								View Workspace
							</DropdownMenuItem>
							<DropdownMenuItem
								className="text-content-destructive focus:text-content-destructive"
								onSelect={handleArchiveAgentAction}
							>
								<ArchiveIcon className="h-3.5 w-3.5" />
								Archive Agent
							</DropdownMenuItem>
						</DropdownMenuContent>
					</DropdownMenu>,
					topBarActionsRef.current,
				)}
			{shouldShowDiffPanel &&
				rightPanelRef?.current &&
				createPortal(<FilesChangedPanel chatId={agentId} />, rightPanelRef.current)}
			<div className="flex h-full flex-col-reverse overflow-y-auto [scrollbar-width:thin] [scrollbar-color:hsl(240_5%_26%)_transparent]">
				<div>
					<div className="mx-auto w-full max-w-3xl py-6">
						{parsedMessages.length === 0 && !hasStreamOutput ? (
							<div className="py-12 text-center text-content-secondary">
								<p className="text-sm">Start a conversation with your agent.</p>
							</div>
						) : (
							<div className="flex flex-col">
								{messageSections.map((section, sectionIdx) => (
									<div
										key={
											section.userEntry?.message.id ?? `section-${sectionIdx}`
										}
									>
										<div className="flex flex-col gap-3">
											{section.entries.map(({ message, parsed }) => {
												const isUser = message.role === "user";
												return isUser ? (
													<div
														key={message.id}
														className="sticky -top-2 z-10 pt-1 drop-shadow-xl"
													>
														<ChatMessageItem
															message={message}
															parsed={parsed}
														/>
													</div>
												) : (
													<ChatMessageItem
														key={message.id}
														message={message}
														parsed={parsed}
													/>
												);
											})}
										</div>
									</div>
								))}

								{hasStreamOutput && (
									<div className="mt-5">
										<StreamingOutput
											streamState={streamState}
											streamTools={streamTools}
										/>
									</div>
								)}
							</div>
						)}

						{detailErrorMessage && (
							<div className="mt-4 rounded-md border border-border-destructive bg-surface-red px-3 py-2 text-xs text-content-destructive">
								{detailErrorMessage}
							</div>
						)}
					</div>

					<AgentChatInput
						onSend={stableOnSend}
						isDisabled={isInputDisabled}
						isLoading={sendMutation.isPending}
						isStreaming={isStreaming}
						onInterrupt={stableOnInterrupt}
						isInterruptPending={interruptMutation.isPending}
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

	return chatContent;
};

export default AgentDetail;
