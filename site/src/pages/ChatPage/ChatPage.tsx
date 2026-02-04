import { API } from "api/api";
import { chat, chatMessages } from "api/queries/chats";
import type { ChatMessage, CreateChatMessageRequest } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { displayError } from "components/GlobalSnackbar/utils";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import {
	PageHeader,
	PageHeaderSubtitle,
	PageHeaderTitle,
} from "components/PageHeader/PageHeader";
import { Spinner } from "components/Spinner/Spinner";
import {
	ArrowLeftIcon,
	SendIcon,
	UserIcon,
	BotIcon,
	WrenchIcon,
} from "lucide-react";
import { type FC, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "react-query";
import { Link as RouterLink, useParams } from "react-router";
import { pageTitle } from "utils/page";
import { OneWayWebSocket } from "utils/OneWayWebSocket";
import type { ServerSentEvent } from "api/typesGenerated";
import { Textarea } from "components/Textarea/Textarea";

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

const ChatPage: FC = () => {
	const { chatId } = useParams() as { chatId: string };
	const queryClient = useQueryClient();
	const chatQuery = useQuery(chat(chatId));
	const messagesQuery = useQuery(chatMessages(chatId));
	const [inputValue, setInputValue] = useState("");
	const [isSending, setIsSending] = useState(false);
	const [streamingText, setStreamingText] = useState<string>("");
	const [currentRunId, setCurrentRunId] = useState<string | null>(null);
	const messagesEndRef = useRef<HTMLDivElement>(null);
	const wsRef = useRef<OneWayWebSocket<ServerSentEvent> | null>(null);

	// Scroll to bottom when messages change or streaming text updates.
	// The dependencies are intentional triggers, not references used in the effect.
	// biome-ignore lint/correctness/useExhaustiveDependencies: Trigger scroll on data changes
	useEffect(() => {
		messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
	}, [messagesQuery.data, streamingText]);

	// Connect to WebSocket for real-time updates
	useEffect(() => {
		if (!chatId) return;

		const ws = new OneWayWebSocket<ServerSentEvent>({
			apiRoute: `/api/v2/chats/${chatId}/messages/watch-ws`,
		});
		wsRef.current = ws;

		ws.addEventListener("message", (event) => {
			if (event.parseError) {
				console.error("WebSocket parse error:", event.parseError);
				return;
			}

			const sse = event.parsedMessage;
			if (!sse || sse.type !== "data") return;

			const data = sse.data;

			// Handle ChatMessage (persisted message from polling)
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
				// Clear streaming text if this message is from the current run
				if (currentRunId) {
					setStreamingText("");
					setCurrentRunId(null);
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
				// Type 0 is text delta in the AI SDK
				if (part.type_id === 0 && typeof part.part === "string") {
					setStreamingText((prev) => prev + part.part);
					setCurrentRunId(part.run_id);
				}
			}
		});

		ws.addEventListener("error", (event) => {
			console.error("WebSocket error:", event);
		});

		return () => {
			ws.close();
			wsRef.current = null;
		};
	}, [chatId, queryClient, currentRunId]);

	const handleSendMessage = async () => {
		if (!inputValue.trim() || isSending) return;

		const content = inputValue.trim();
		setInputValue("");
		setIsSending(true);
		setStreamingText("");

		try {
			const req: CreateChatMessageRequest = { content };
			const response = await API.createChatMessage(chatId, req);
			setCurrentRunId(response.run_id);
			// The message will appear via WebSocket
		} catch {
			displayError("Failed to send message");
		} finally {
			setIsSending(false);
		}
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
	const messages = messagesQuery.data || [];

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

				<main className="flex flex-col h-[calc(100vh-200px)]">
					{/* Messages area */}
					<div className="flex-1 overflow-y-auto pb-4 space-y-4">
						{messages.map((msg) => {
							// Skip system messages in the UI (they're internal)
							if (msg.role === "system") return null;

							const { text, toolCalls } = parseMessageContent(msg);

							return (
								<div
									key={msg.id}
									className={`flex gap-3 ${
										msg.role === "user" ? "justify-end" : "justify-start"
									}`}
								>
									<div
										className={`flex gap-3 max-w-[80%] ${
											msg.role === "user" ? "flex-row-reverse" : "flex-row"
										}`}
									>
										<div
											className={`flex items-center justify-center size-8 rounded-full shrink-0 ${
												msg.role === "user"
													? "bg-surface-secondary"
													: "bg-surface-tertiary"
											}`}
										>
											{getRoleIcon(msg.role)}
										</div>
										<div className="flex flex-col gap-1">
											<span className="text-xs text-content-secondary">
												{getRoleLabel(msg.role)}
											</span>
											<div
												className={`rounded-lg px-4 py-2 ${
													msg.role === "user"
														? "bg-surface-secondary"
														: "bg-surface-tertiary"
												}`}
											>
												{text && (
													<p className="whitespace-pre-wrap text-sm">{text}</p>
												)}
												{toolCalls.length > 0 && (
													<div className="mt-2 space-y-2">
														{toolCalls.map((tc, idx) => (
															<details
																key={idx}
																className="text-xs bg-surface-primary rounded p-2"
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
							<div className="flex gap-3 justify-start">
								<div className="flex gap-3 max-w-[80%]">
									<div className="flex items-center justify-center size-8 rounded-full shrink-0 bg-surface-tertiary">
										<BotIcon className="size-5" />
									</div>
									<div className="flex flex-col gap-1">
										<span className="text-xs text-content-secondary">
											Assistant
										</span>
										<div className="rounded-lg px-4 py-2 bg-surface-tertiary">
											<p className="whitespace-pre-wrap text-sm">
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
					<div className="border-t border-border pt-4">
						<div className="flex gap-2">
							<Textarea
								value={inputValue}
								onChange={(e) => setInputValue(e.target.value)}
								onKeyDown={handleKeyDown}
								placeholder="Type your message... (Enter to send, Shift+Enter for new line)"
								className="flex-1 min-h-[60px] max-h-[200px] resize-none"
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
					</div>
				</main>
			</Margins>
		</>
	);
};

export default ChatPage;
