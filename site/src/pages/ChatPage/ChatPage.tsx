import { API } from "api/api";
import { chat, chatMessages } from "api/queries/chats";
import { template as templateQueryOptions } from "api/queries/templates";
import { workspaceById } from "api/queries/workspaces";
import type {
	ChatMessage,
	CreateChatMessageRequest,
	ServerSentEvent,
} from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { ScrollArea } from "components/ScrollArea/ScrollArea";
import { Spinner } from "components/Spinner/Spinner";
import { Textarea } from "components/Textarea/Textarea";
import { useWorkspaceBuildLogs } from "hooks/useWorkspaceBuildLogs";
import {
	ArrowLeftIcon,
	BotIcon,
	SendIcon,
	UserIcon,
	WrenchIcon,
} from "lucide-react";
import { WorkspaceBuildLogs } from "modules/workspaces/WorkspaceBuildLogs/WorkspaceBuildLogs";
import {
	WorkspaceBuildProgress,
	getActiveTransitionStats,
} from "pages/WorkspacePage/WorkspaceBuildProgress";
import {
	type FC,
	useCallback,
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
} from "react";
import { useQuery, useQueryClient } from "react-query";
import { Link as RouterLink, useParams, useSearchParams } from "react-router";
import { OneWayWebSocket } from "utils/OneWayWebSocket";

import { pageTitle } from "utils/page";

// Message envelope types from the backend
interface MessageEnvelope {
	type: "message" | "system_prompt";
	run_id?: string;
	message?: {
		role: string;
		parts: Array<{
			type: string;
			text?: string;
			toolCallId?: string;
			toolName?: string;
			args?: Record<string, unknown>;
			result?: unknown;
		}>;
	};
	content?: string;
}

// Streaming part from WebSocket
interface StreamingPart {
	run_id: string;
	type_id: number;
	part: unknown;
}

const textDeltaTypeID = "0".charCodeAt(0);

const dedupeMessages = (messages: ChatMessage[]) => {
	const seen = new Set<number>();
	return messages.filter((msg) => {
		if (seen.has(msg.id)) {
			return false;
		}
		seen.add(msg.id);
		return true;
	});
};

const getMessageRunId = (msg: ChatMessage): string | null => {
	try {
		const envelope = JSON.parse(
			JSON.stringify(msg.content),
		) as MessageEnvelope;
		if (envelope.type === "message" && envelope.run_id) {
			return envelope.run_id;
		}
	} catch {
		return null;
	}
	return null;
};

const ChatPage: FC = () => {
	const { chatId } = useParams() as { chatId: string };
	const [searchParams, setSearchParams] = useSearchParams();
	const queryClient = useQueryClient();
	const [pendingRunId, setPendingRunId] = useState<string | null>(null);
	const chatQuery = useQuery(chat(chatId));
	const messagesQuery = useQuery({
		...chatMessages(chatId),
		refetchInterval: pendingRunId ? 2_000 : false,
	});
	const dedupedMessages = dedupeMessages(messagesQuery.data || []);
	const workspaceId = chatQuery.data?.workspace_id ?? undefined;
	const workspaceQuery = useQuery({
		...workspaceById(workspaceId ?? ""),
		enabled: Boolean(workspaceId),
		refetchInterval: ({ state }) => {
			const status = state.data?.latest_build.status;
			return status && status !== "running" ? 2_000 : false;
		},
	});
	const workspace = workspaceQuery.data;

	const templateQuery = useQuery({
		...templateQueryOptions(workspace?.template_id ?? ""),
		enabled: Boolean(workspace),
	});
	const template = templateQuery.data;

	const workspaceStatus = workspace?.latest_build.status;
	const shouldShowBuildLogs = Boolean(
		workspaceStatus &&
			workspaceStatus !== "running" &&
			workspaceStatus !== "stopped" &&
			workspaceStatus !== "deleted",
	);
	const buildLogs = useWorkspaceBuildLogs(
		workspace?.latest_build.id,
		Boolean(workspace?.latest_build.id),
	);
	const showBuildLogs = shouldShowBuildLogs || buildLogs !== undefined;
	const [inputValue, setInputValue] = useState("");
	const [isSending, setIsSending] = useState(false);
	const isSendingRef = useRef(false);
	const [streamingText, setStreamingText] = useState<string>("");
	const currentRunIdRef = useRef<string | null>(null);
	const initialMessageSentRef = useRef(false);
	const messagesEndRef = useRef<HTMLDivElement>(null);
	const buildLogsScrollRef = useRef<HTMLDivElement>(null);
	const wsRef = useRef<OneWayWebSocket<ServerSentEvent> | null>(null);
	const setRunId = useCallback((runId: string | null) => {
		currentRunIdRef.current = runId;
	}, []);

	const sendMessage = useCallback(
		async (content: string) => {
			const trimmed = content.trim();
			if (!trimmed || isSendingRef.current) return;

			isSendingRef.current = true;
			setIsSending(true);
			setStreamingText("");
			setRunId(null);

			try {
				const req: CreateChatMessageRequest = { content: trimmed };
				const response = await API.createChatMessage(chatId, req);
				setRunId(response.run_id);
				setPendingRunId(response.run_id);
				queryClient.invalidateQueries({
					queryKey: ["chat", chatId, "messages"],
				});
			} catch {
				displayError("Failed to send message");
			} finally {
				isSendingRef.current = false;
				setIsSending(false);
			}
		},
		[chatId, queryClient, setPendingRunId, setRunId],
	);

	// Scroll to bottom when messages change or streaming text updates.
	// The dependencies are intentional triggers, not references used in the effect.
	// biome-ignore lint/correctness/useExhaustiveDependencies: Trigger scroll on data changes
	useEffect(() => {
		messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
	}, [messagesQuery.data, streamingText]);

	// If we have a pending run and miss WebSocket events, poll until the
	// persisted assistant message is visible.
	useEffect(() => {
		if (!pendingRunId || !messagesQuery.data) {
			return;
		}
		const hasAssistantForRun = messagesQuery.data.some((msg) => {
			if (msg.role !== "assistant") {
				return false;
			}
			const runId = getMessageRunId(msg);
			return !runId || runId === pendingRunId;
		});
		if (hasAssistantForRun) {
			setPendingRunId(null);
			setStreamingText("");
			setRunId(null);
		}
	}, [messagesQuery.data, pendingRunId, setRunId]);

	// Keep build logs scrolled to the latest entry while logs are visible.
	// biome-ignore lint/correctness/useExhaustiveDependencies: Trigger on log updates
	useLayoutEffect(() => {
		if (!showBuildLogs || buildLogs === undefined) {
			return;
		}
		const scrollAreaEl = buildLogsScrollRef.current;
		const scrollViewportEl = scrollAreaEl?.querySelector<HTMLDivElement>(
			"[data-radix-scroll-area-viewport]",
		);
		if (scrollViewportEl) {
			scrollViewportEl.scrollTop = scrollViewportEl.scrollHeight;
		}
	}, [buildLogs, showBuildLogs]);

	useEffect(() => {
		initialMessageSentRef.current = false;
		setPendingRunId(null);
		setStreamingText("");
		setRunId(null);
	}, [chatId, setRunId]);

	// Handle initial message from URL query param
	useEffect(() => {
		const initialMessage = searchParams.get("message");
		if (!initialMessage?.trim() || initialMessageSentRef.current || isSending) {
			return;
		}

		initialMessageSentRef.current = true;
		setSearchParams({}, { replace: true });
		void sendMessage(initialMessage);
	}, [searchParams, setSearchParams, sendMessage, isSending]);

	// Connect to WebSocket for real-time updates with automatic reconnection.
	useEffect(() => {
		if (!chatId) return;

		let cancelled = false;
		let reconnectAttempts = 0;
		let reconnectTimeout: ReturnType<typeof setTimeout> | null = null;
		const maxReconnectDelay = 30_000;

		const connect = () => {
			if (cancelled) return;

			const ws = new OneWayWebSocket<ServerSentEvent>({
				apiRoute: `/api/v2/chats/${chatId}/messages/watch-ws`,
			});
			wsRef.current = ws;

			ws.addEventListener("open", () => {
				// Reset backoff on successful connection.
				reconnectAttempts = 0;
				// Immediately refresh messages in case we missed events.
				queryClient.invalidateQueries({
					queryKey: ["chat", chatId, "messages"],
				});
			});

			ws.addEventListener("message", (event) => {
				if (event.parseError) {
					console.error("WebSocket parse error:", event.parseError);
					return;
				}

				const sse = event.parsedMessage;
				if (!sse || sse.type !== "data") return;

				const data = sse.data;

				// Handle ChatMessage (persisted message from the server)
				if (
					data &&
					typeof data === "object" &&
					"chat_id" in data &&
					"role" in data
				) {
					// New persisted message - refresh the query
					queryClient.invalidateQueries({
						queryKey: ["chat", chatId, "messages"],
					});
					const message = data as ChatMessage;
					if (message.role === "assistant") {
						const messageRunId = getMessageRunId(message);
						setPendingRunId((prev) => {
							if (!prev) {
								return prev;
							}
							if (!messageRunId || prev === messageRunId) {
								return null;
							}
							return prev;
						});
						if (!messageRunId || messageRunId === currentRunIdRef.current) {
							setStreamingText("");
							setRunId(null);
						}
					}
					return;
				}

				// Handle streaming part (real-time from LLM)
				if (
					data &&
					typeof data === "object" &&
					"run_id" in data &&
					"type_id" in data
				) {
					const part = data as StreamingPart;
					// Text deltas use type ID "0" (ASCII 48) in the AI SDK stream.
					if (part.type_id === textDeltaTypeID) {
						let delta: string | undefined;
						if (typeof part.part === "string") {
							delta = part.part;
						} else if (part.part && typeof part.part === "object") {
							const maybeContent =
								(part.part as { Content?: string; content?: string }).Content ??
								(part.part as { content?: string }).content;
							if (typeof maybeContent === "string") {
								delta = maybeContent;
							}
						}
						if (delta) {
							setStreamingText((prev) => prev + delta);
							setRunId(part.run_id);
							setPendingRunId(part.run_id);
						}
					}
				}
			});

			ws.addEventListener("close", () => {
				if (cancelled) return;
				// Schedule reconnection with exponential backoff.
				const delay = Math.min(1000 * 2 ** reconnectAttempts, maxReconnectDelay);
				reconnectAttempts++;
				reconnectTimeout = setTimeout(connect, delay);
			});

			ws.addEventListener("error", (event) => {
				console.error("WebSocket error:", event);
			});
		};

		connect();

		return () => {
			cancelled = true;
			if (reconnectTimeout) {
				clearTimeout(reconnectTimeout);
			}
			wsRef.current?.close();
			wsRef.current = null;
		};
	}, [chatId, queryClient, setRunId]);

	const handleSendMessage = async () => {
		if (isSending) return;

		const content = inputValue.trim();
		if (!content) return;

		setInputValue("");
		await sendMessage(content);
	};

	const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
		if (e.key === "Enter" && !e.shiftKey) {
			e.preventDefault();
			handleSendMessage();
		}
	};

	// Parse message content from envelope
	const parseMessageContent = (
		msg: ChatMessage,
	): {
		text: string;
		toolCalls: Array<{ name: string; args: unknown; result?: unknown }>;
	} => {
		try {
			const envelope = JSON.parse(
				JSON.stringify(msg.content),
			) as MessageEnvelope;

			// System prompt
			if (envelope.type === "system_prompt") {
				return { text: envelope.content || "", toolCalls: [] };
			}

			// Regular message
			if (envelope.message?.parts) {
				const textParts: string[] = [];
				const toolCalls: Array<{
					name: string;
					args: unknown;
					result?: unknown;
				}> = [];

				for (const part of envelope.message.parts) {
					if (part.type === "text" && part.text) {
						textParts.push(part.text);
					} else if (part.type === "tool-call" && part.toolName) {
						toolCalls.push({ name: part.toolName, args: part.args || {} });
					} else if (part.type === "tool-result" && part.toolName) {
						// Find matching tool call and add result
						const existing = toolCalls.find((tc) => tc.name === part.toolName);
						if (existing) {
							existing.result = part.result;
						} else {
							toolCalls.push({
								name: part.toolName,
								args: {},
								result: part.result,
							});
						}
					}
				}

				return { text: textParts.join("\n"), toolCalls };
			}

			// Fallback: try to display raw content
			return { text: JSON.stringify(msg.content), toolCalls: [] };
		} catch {
			return { text: String(msg.content), toolCalls: [] };
		}
	};

	const getRoleIcon = (role: string) => {
		switch (role) {
			case "user":
				return <UserIcon className="size-5" />;
			case "assistant":
				return <BotIcon className="size-5" />;
			case "system":
				return <WrenchIcon className="size-5" />;
			default:
				return <BotIcon className="size-5" />;
		}
	};

	const getRoleLabel = (role: string) => {
		switch (role) {
			case "user":
				return "You";
			case "assistant":
				return "Assistant";
			case "system":
				return "System";
			default:
				return role;
		}
	};

	if (chatQuery.isLoading || messagesQuery.isLoading) {
		return (
			<Margins>
				<title>{pageTitle("Loading Chat")}</title>
				<Loader className="h-screen" />
			</Margins>
		);
	}

	if (chatQuery.isError || !chatQuery.data) {
		return (
			<Margins>
				<title>{pageTitle("Chat Not Found")}</title>
				<div className="flex flex-col items-center justify-center py-16">
					<h2 className="text-lg font-medium">Chat not found</h2>
					<p className="text-content-secondary text-sm mt-2">
						The chat you're looking for doesn't exist or you don't have access
						to it.
					</p>
					<Button variant="outline" asChild className="mt-4">
						<RouterLink to="/chats">
							<ArrowLeftIcon className="size-4" />
							Back to Chats
						</RouterLink>
					</Button>
				</div>
			</Margins>
		);
	}

	const chatData = chatQuery.data;
	const transitionStats = workspace
		? (template && getActiveTransitionStats(template, workspace)) || {
				P50: 0,
				P95: null,
			}
		: undefined;
	const workspaceStatusLabel = workspaceStatus ?? "starting";
	const workspaceName = workspace?.name ?? "workspace";

	return (
		<>
			<title>{pageTitle(chatData.title || "Chat")}</title>
			<Margins>
				<PageHeader
					actions={
						<Button variant="outline" asChild>
							<RouterLink to="/chats">
								<ArrowLeftIcon className="size-4" />
								Back to Chats
							</RouterLink>
						</Button>
					}
				>
					<PageHeaderTitle>{chatData.title || "Untitled Chat"}</PageHeaderTitle>
					<PageHeaderSubtitle>
						{chatData.provider} / {chatData.model}
					</PageHeaderSubtitle>
				</PageHeader>

				<main className="flex flex-col h-[calc(100vh-200px)] pb-8 gap-6">
					{showBuildLogs && workspace && (
						<section className="rounded-xl border border-border bg-surface-secondary p-4 shadow-sm">
							<div className="flex flex-col gap-4">
								<div className="flex flex-col gap-1">
									<p className="m-0 text-sm font-medium text-content-primary">
										Workspace {workspaceStatusLabel}...
									</p>
									<p className="m-0 text-xs text-content-secondary">
										Tools will be available once {workspaceName} is running.
									</p>
								</div>

								{transitionStats && (
									<WorkspaceBuildProgress
										workspace={workspace}
										transitionStats={transitionStats}
									/>
								)}

								<div className="rounded-lg border border-border bg-surface-primary">
									{buildLogs === undefined ? (
										<div className="h-64">
											<Loader />
										</div>
									) : (
										<ScrollArea ref={buildLogsScrollRef} className="h-64">
											<WorkspaceBuildLogs
												sticky
												className="border-0 rounded-none"
												logs={buildLogs}
											/>
										</ScrollArea>
									)}
								</div>
							</div>
						</section>
					)}
					<div className="flex flex-1 flex-col rounded-xl border border-border bg-surface-primary shadow-sm overflow-hidden">
					{/* Messages area */}
					<div className="flex-1 overflow-y-auto px-6 py-6 space-y-6">
						{dedupedMessages.map((msg) => {
							// Skip system messages in the UI (they're internal)
							if (msg.role === "system") return null;

							const { text, toolCalls } = parseMessageContent(msg);
							const isUser = msg.role === "user";

							return (
								<div
									key={msg.id}
									className={`flex ${
										isUser ? "justify-end" : "justify-start"
									}`}
								>
									<div
										className={`flex max-w-[75%] gap-3 ${
											isUser ? "flex-row-reverse" : "flex-row"
										}`}
									>
										<div
											className={`flex size-9 shrink-0 items-center justify-center rounded-full border border-border ${
												isUser
													? "bg-surface-invert-primary text-content-invert border-transparent"
													: "bg-surface-secondary text-content-secondary"
											}`}
										>
											{getRoleIcon(msg.role)}
										</div>
										<div className="flex flex-col gap-1">
											<span className="text-[11px] uppercase tracking-wide text-content-secondary">
												{getRoleLabel(msg.role)}
											</span>
											<div
												className={`rounded-2xl px-4 py-3 text-sm leading-relaxed shadow-sm ${
													isUser
														? "bg-surface-invert-primary text-content-invert"
														: "bg-surface-secondary text-content-primary border border-border"
												}`}
											>
												{text && (
													<p className="whitespace-pre-wrap text-sm leading-relaxed">{text}</p>
												)}
												{toolCalls.length > 0 && (
													<div className="mt-3 space-y-2 text-content-primary">
														{toolCalls.map((tc, idx) => (
															<details
																key={idx}
																className="text-xs rounded-md border border-border bg-surface-primary p-2 text-content-primary"
															>
																<summary className="cursor-pointer flex items-center gap-1 text-content-secondary">
																	<WrenchIcon className="size-3" />
																	{tc.name}
																</summary>
																<div className="mt-2 space-y-1">
																	<div>
																		<span className="font-medium">Args:</span>
																		<pre className="text-xs overflow-x-auto">
																			{JSON.stringify(tc.args, null, 2)}
																		</pre>
																	</div>
																	{tc.result !== undefined && (
																		<div>
																			<span className="font-medium">
																				Result:
																			</span>
																			<pre className="text-xs overflow-x-auto max-h-32">
																				{typeof tc.result === "string"
																					? tc.result
																					: JSON.stringify(tc.result, null, 2)}
																			</pre>
																		</div>
																	)}
																</div>
															</details>
														))}
													</div>
												)}
											</div>
										</div>
									</div>
								</div>
							);
						})}

						{/* Streaming response */}
						{streamingText && (
							<div className="flex justify-start">
								<div className="flex max-w-[75%] gap-3">
									<div className="flex size-9 shrink-0 items-center justify-center rounded-full border border-border bg-surface-secondary text-content-secondary">
										<BotIcon className="size-5" />
									</div>
									<div className="flex flex-col gap-1">
										<span className="text-[11px] uppercase tracking-wide text-content-secondary">
											Assistant
										</span>
										<div className="rounded-2xl px-4 py-3 text-sm leading-relaxed shadow-sm border border-border bg-surface-secondary text-content-primary">
											<p className="whitespace-pre-wrap text-sm leading-relaxed">
												{streamingText}
												<span className="animate-pulse">▌</span>
											</p>
										</div>
									</div>
								</div>
							</div>
						)}

						<div ref={messagesEndRef} />
					</div>

					{/* Input area */}
					<div className="border-t border-border bg-surface-secondary p-4">
						<div className="flex flex-col gap-3 md:flex-row md:items-end">
							<Textarea
								value={inputValue}
								onChange={(e) => setInputValue(e.target.value)}
								onKeyDown={handleKeyDown}
								placeholder="Type your message... (Enter to send, Shift+Enter for new line)"
								className="flex-1 min-h-[80px] max-h-[200px] resize-none bg-surface-primary"
								disabled={isSending}
							/>
							<Button
								onClick={handleSendMessage}
								disabled={!inputValue.trim() || isSending}
								className="self-end"
							>
								<Spinner loading={isSending}>
									<SendIcon className="size-4" />
								</Spinner>
								Send
							</Button>
						</div>
						<p className="mt-2 text-xs text-content-secondary">
							Press Enter to send. Shift+Enter for a new line.
						</p>
					</div>
					</div>
				</main>
			</Margins>
		</>
	);
};

export default ChatPage;
