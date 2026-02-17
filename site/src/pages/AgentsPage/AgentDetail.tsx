import { type FC, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useOutletContext, useParams } from "react-router";
import TextareaAutosize from "react-textarea-autosize";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { type ChatDiffStatusResponse, watchChat } from "api/api";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import {
	chat,
	chatDiffContentsKey,
	chatDiffStatus,
	chatDiffStatusKey,
	chatModels,
	chatsKey,
	createChatMessage,
	interruptChat,
} from "api/queries/chats";
import type * as TypesGen from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Loader } from "components/Loader/Loader";
import {
	Conversation,
	ConversationItem,
	Message,
	MessageContent,
	ModelSelector,
	Response,
	Thinking,
	Tool,
	type ModelSelectorOption,
} from "components/ai-elements";
import {
	PanelRightCloseIcon,
	PanelRightOpenIcon,
	SendIcon,
	Square,
} from "lucide-react";
import type { OneWayMessageEvent } from "utils/OneWayWebSocket";
import type { AgentsOutletContext } from "./AgentsPage";
import { FilesChangedPanel } from "./FilesChangedPanel";
import {
	formatProviderLabel,
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
			status: result
				? result.isError
					? "error"
					: "completed"
				: "completed",
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
					parsed.markdown = appendText(parsed.markdown, asString(typedBlock.text));
					break;
				case "reasoning":
				case "thinking":
					parsed.reasoning = appendText(parsed.reasoning, asString(typedBlock.text));
					break;
				case "tool-call":
				case "toolcall": {
					const name = asString(typedBlock.tool_name) || asString(typedBlock.name);
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
					const name = asString(typedBlock.tool_name) || asString(typedBlock.name);
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
					parsed.markdown = appendText(parsed.markdown, asString(typedBlock.text));
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
				option.model === model &&
				(!provider || option.provider === provider),
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
	parsed.tools = mergeTools(
		parsed.toolCalls,
		Array.from(resultById.values()),
	);
	return parsed;
};

type CreateChatMessagePayload = TypesGen.CreateChatMessageRequest & {
	readonly model?: string;
};

const noopSetChatErrorReason: AgentsOutletContext["setChatErrorReason"] = () => {};
const noopClearChatErrorReason: AgentsOutletContext["clearChatErrorReason"] =
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
			className="absolute right-3 top-3 z-10 flex cursor-pointer items-center gap-3 px-2 py-1 text-content-secondary transition-colors hover:text-content-primary"
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

export const AgentDetail: FC = () => {
	const { agentId } = useParams<{ agentId: string }>();
	const outletContext = useOutletContext<AgentsOutletContext | undefined>();
	const queryClient = useQueryClient();
	const [input, setInput] = useState("");
	const [messagesById, setMessagesById] = useState<
		Map<number, TypesGen.ChatMessage>
	>(new Map());
	const [streamState, setStreamState] = useState<StreamState | null>(null);
	const [streamError, setStreamError] = useState<string | null>(null);
	const [chatStatus, setChatStatus] = useState<TypesGen.ChatStatus | null>(null);
	const [selectedModel, setSelectedModel] = useState("");
	const chatErrorReasons = outletContext?.chatErrorReasons ?? {};
	const setChatErrorReason =
		outletContext?.setChatErrorReason ?? noopSetChatErrorReason;
	const clearChatErrorReason =
		outletContext?.clearChatErrorReason ?? noopClearChatErrorReason;
	const streamResetFrameRef = useRef<number | null>(null);

	const chatQuery = useQuery({
		...chat(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const diffStatusQuery = useQuery({
		...chatDiffStatus(agentId ?? ""),
		enabled: Boolean(agentId),
	});
	const chatModelsQuery = useQuery(chatModels());
	const hasDiffStatus = Boolean(diffStatusQuery.data?.pull_request_url);
	const [showDiffPanel, setShowDiffPanel] = useState(false);

	// Auto-open the diff panel when diff status becomes available.
	useEffect(() => {
		if (hasDiffStatus) {
			setShowDiffPanel(true);
		}
	}, [hasDiffStatus]);

	const catalogModelOptions = useMemo(
		() => getModelOptionsFromCatalog(chatModelsQuery.data),
		[chatModelsQuery.data],
	);
	const modelOptions = catalogModelOptions;

	const sendMutation = useMutation(createChatMessage(queryClient, agentId ?? ""));
	const interruptMutation = useMutation(interruptChat(queryClient, agentId ?? ""));
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
			return resolveModelFromChatConfig(chatQuery.data.chat.model_config, modelOptions);
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

					switch (normalizeBlockType(part.type)) {
						case "text":
							setStreamState((prev) => {
								const nextState: StreamState = prev ?? createEmptyStreamState();
								return {
									...nextState,
									content: `${nextState.content}${asString(part.text)}`,
								};
							});
							return;
						case "reasoning":
						case "thinking":
							setStreamState((prev) => {
								const nextState: StreamState = prev ?? createEmptyStreamState();
								return {
									...nextState,
									reasoning: `${nextState.reasoning}${asString(part.text)}`,
								};
							});
							return;
						case "tool-call":
						case "toolcall": {
							const toolName = asString(part.tool_name);

							setStreamState((prev) => {
								const nextState: StreamState = prev ?? createEmptyStreamState();
								const existingByName = Object.values(nextState.toolCalls).find(
									(call) => call.name === toolName,
								);
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
							return;
						}
						case "tool-result":
						case "toolresult": {
							const toolName = asString(part.tool_name);

							setStreamState((prev) => {
								const nextState: StreamState = prev ?? createEmptyStreamState();
								const existingByName = Object.values(nextState.toolResults).find(
									(result) => result.name === toolName,
								);
								const existingCallByName = Object.values(nextState.toolCalls).find(
									(call) => call.name === toolName,
								);
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
							const shouldRefreshQueries =
								nextStatus === "completed" ||
								nextStatus === "error" ||
								nextStatus === "paused" ||
								nextStatus === "waiting";
							if (agentId && shouldRefreshQueries) {
								void Promise.all([
									queryClient.invalidateQueries({
										queryKey: chatDiffStatusKey(agentId),
									}),
									queryClient.invalidateQueries({
										queryKey: chatDiffContentsKey(agentId),
									}),
								]);
							}
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
		list.sort((a, b) =>
			new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
		);
		return list;
	}, [messagesById]);

	const isStreaming =
		Boolean(streamState) || chatStatus === "running" || chatStatus === "pending";
	const hasModelOptions = modelOptions.length > 0;
	const hasConfiguredModels = hasConfiguredModelsInCatalog(chatModelsQuery.data);
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
	const isSubmissionPending = sendMutation.isPending || interruptMutation.isPending;
	const isInputDisabled = isSubmissionPending || !hasModelOptions;

	const handleSend = async () => {
		const prompt = input;
		if (!prompt.trim() || isSubmissionPending || !agentId || !hasModelOptions) {
			return;
		}
		if (isStreaming) {
			await interruptMutation.mutateAsync();
		}

		// Content is a JSON raw message; preserve plain text message shape.
		const request: CreateChatMessagePayload = {
			role: "user",
			content: JSON.parse(JSON.stringify(prompt)),
			model: selectedModel || undefined,
		};

		clearChatErrorReason(agentId);
		setStreamError(null);
		await sendMutation.mutateAsync(request);
		setInput("");
	};

	const handleInterrupt = async () => {
		if (!agentId || interruptMutation.isPending) {
			return;
		}
		await interruptMutation.mutateAsync();
	};

	const handleKeyDown = (e: React.KeyboardEvent) => {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			void handleSend();
		}
	};

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
				status: result
					? result.isError
						? "error"
						: "completed"
					: "running",
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

	if (chatQuery.isLoading) {
		return (
			<div className="flex flex-1 items-center justify-center">
				<Loader />
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

	const chatContent = (
		<div className="relative flex h-full min-h-0 min-w-0 flex-1 flex-col">
			{hasDiffStatus && diffStatusQuery.data && (
				<DiffStatsBadge
					status={diffStatusQuery.data}
					isOpen={showDiffPanel}
					onToggle={() => setShowDiffPanel((prev) => !prev)}
				/>
			)}
			<div className="flex h-full flex-col-reverse overflow-y-auto [scrollbar-width:thin]">
				<div>
					<div className="mx-auto w-full max-w-3xl py-6">
						{visibleMessages.length === 0 && !hasStreamOutput ? (
							<div className="py-12 text-center text-content-secondary">
								<p className="text-sm">Start a conversation with your agent.</p>
							</div>
						) : (
							<Conversation>
								{visibleMessages.map((message) => {
									const parsed = parseChatMessageContent(message, globalToolResults);
									const isUser = message.role === "user";

									// Skip messages that only carry tool results.
									// Those results are now shown inline with the
									// tool-call message they belong to.
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

									return (
										<ConversationItem
											key={message.id}
											role={isUser ? "user" : "assistant"}
										>
											{isUser ? (
												<Message className="max-w-[min(44rem,78%)]">
													<MessageContent className="rounded-2xl border border-border-default/60 bg-surface-tertiary/75 px-4 py-2.5 font-sans shadow-sm">
														{parsed.markdown || ""}
													</MessageContent>
												</Message>
											) : (
												<Message>
													<MessageContent className="whitespace-normal">
														<div className="space-y-3">
															{parsed.markdown && (
																<Response>{parsed.markdown}</Response>
															)}
															{parsed.reasoning && (
																<Thinking>{parsed.reasoning}</Thinking>
															)}
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
								})}

								{hasStreamOutput && (
									<ConversationItem role="assistant">
										<Message>
											<MessageContent className="whitespace-normal">
												<div className="space-y-3">
													{streamState?.content ? (
														<Response>{streamState.content}</Response>
													) : (
														<div className="text-sm text-content-secondary">
															Agent is thinking...
														</div>
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
								)}
							</Conversation>
						)}

						{detailErrorMessage && (
							<div className="mt-4 rounded-md border border-border-destructive bg-surface-red px-3 py-2 text-xs text-content-destructive">
								{detailErrorMessage}
							</div>
						)}
					</div>

					<div className="sticky bottom-0 z-50 bg-surface-primary">
						<div className="mx-auto w-full max-w-3xl pb-4">
							<div className="rounded-2xl border border-border-default/80 bg-surface-secondary/45 p-1 shadow-sm focus-within:ring-2 focus-within:ring-content-link/40">
								<TextareaAutosize
									className="min-h-[120px] w-full resize-none border-none bg-transparent px-3 py-2 font-sans text-[15px] leading-6 text-content-primary outline-none placeholder:text-content-secondary disabled:cursor-not-allowed disabled:opacity-70"
									placeholder="Type a message..."
									value={input}
									onChange={(e) => setInput(e.target.value)}
									onKeyDown={handleKeyDown}
									disabled={isInputDisabled}
									minRows={4}
								/>
							<div className="flex items-center justify-between gap-2 px-2.5 pb-1.5">
								<div className="flex min-w-0 items-center gap-2">
									<ModelSelector
										value={selectedModel}
										onValueChange={setSelectedModel}
										options={modelOptions}
										disabled={isInputDisabled}
										placeholder={modelSelectorPlaceholder}
										formatProviderLabel={formatProviderLabel}
										dropdownSide="top"
										dropdownAlign="start"
										className="h-8 justify-start border-none bg-transparent px-1 text-xs shadow-none hover:bg-transparent"
									/>
									{inputStatusText && (
										<span className="hidden text-xs text-content-secondary sm:inline">
											{inputStatusText}
										</span>
									)}
								</div>
								<div className="flex items-center gap-2">
									{isStreaming && (
										<Button
											size="icon"
											variant="outline"
											className="rounded-full"
											onClick={() => void handleInterrupt()}
											disabled={interruptMutation.isPending}
										>
											<Square className="h-4 w-4" />
											<span className="sr-only">Interrupt</span>
										</Button>
									)}
									<Button
										size="icon"
										variant="default"
										className="rounded-full"
										onClick={() => void handleSend()}
										disabled={isInputDisabled || !hasModelOptions || !input.trim()}
									>
										<SendIcon />
										<span className="sr-only">Send</span>
									</Button>
								</div>
							</div>
							{inputStatusText && (
								<div className="px-2.5 pb-1 text-xs text-content-secondary sm:hidden">
									{inputStatusText}
								</div>
							)}
							{modelCatalogStatusMessage && (
								<div className="px-2.5 pb-1 text-2xs text-content-secondary">
									{modelCatalogStatusMessage}
								</div>
							)}
							</div>
						</div>
					</div>
				</div>
			</div>
		</div>
	);

	if (hasDiffStatus && showDiffPanel) {
		return (
			<PanelGroup
				autoSaveId="agent-diff"
				direction="horizontal"
				className="h-full min-h-0"
			>
				<Panel defaultSize={65} minSize={40}>
					{chatContent}
				</Panel>
				<PanelResizeHandle className="group flex w-0 items-stretch">
					<div className="w-[3px] bg-border-default transition-colors group-hover:bg-content-link group-data-[resize-handle-active]:bg-content-link" />
				</PanelResizeHandle>
				<Panel defaultSize={35} minSize={20} className="hidden xl:block">
					<FilesChangedPanel chatId={agentId} />
				</Panel>
			</PanelGroup>
		);
	}

	return chatContent;
};

export default AgentDetail;
