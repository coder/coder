import type { Workspace } from "api/typesGenerated";
import { Button } from "components/Button/Button";
import { Spinner } from "components/Spinner/Spinner";
import { useAppLink } from "modules/apps/useAppLink";
import type { WorkspaceAppWithAgent } from "modules/tasks/apps";
import {
	type FC,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import { cn } from "utils/cn";
import { useAcpClient } from "use-acp";
import type { NotificationEvent } from "use-acp";

type TaskAcpChatProps = {
	workspace: Workspace;
	app: WorkspaceAppWithAgent;
	active: boolean;
};

export const TaskAcpChat: FC<TaskAcpChatProps> = ({
	workspace,
	app,
	active,
}) => {
	const link = useAppLink(app, {
		agent: app.agent,
		workspace,
	});

	// Convert HTTP URL to WebSocket URL
	const wsUrl = constructWsUrl(link.href);

	const {
		connect,
		connectionState,
		notifications,
		pendingPermission,
		resolvePermission,
		agent: acpAgent,
		activeSessionId,
		setActiveSessionId,
	} = useAcpClient({
		wsUrl,
		autoConnect: false, // Manual control for health checks
		reconnectAttempts: 3,
		reconnectDelay: 2000,
	});

	const [promptText, setPromptText] = useState("");
	const [isPrompting, setIsPrompting] = useState(false);
	const messagesEndRef = useRef<HTMLDivElement>(null);

	// Auto-approve permissions for POC
	useEffect(() => {
		if (pendingPermission && pendingPermission.options.length > 0) {
			resolvePermission({
				outcome: {
					outcome: "selected",
					optionId: pendingPermission.options[0].optionId,
				},
			});
		}
	}, [pendingPermission, resolvePermission]);

	// Create session when connected
	useEffect(() => {
		const createSession = async () => {
			if (
				connectionState.status === "connected" &&
				acpAgent &&
				!activeSessionId
			) {
				try {
					const response = await acpAgent.newSession({
						cwd: "/tmp",
						mcpServers: [],
					});
					// biome-ignore lint/suspicious/noExplicitAny: SessionId is a branded type not exported from use-acp
					setActiveSessionId(response.sessionId as any);
				} catch (error) {
					console.error("Failed to create ACP session:", error);
				}
			}
		};

		void createSession();
	}, [connectionState.status, acpAgent, activeSessionId, setActiveSessionId]);

	// Merge consecutive message chunks into single messages
	const mergedMessages = useMemo(() => {
		return mergeMessageChunks(notifications);
	}, [notifications]);

	// Auto-scroll to bottom when new messages arrive
	// biome-ignore lint/correctness/useExhaustiveDependencies: We want to scroll when notifications change
	useEffect(() => {
		messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
	}, [notifications]);

	// Connect when app becomes healthy and active
	useEffect(() => {
		if (active && (app.health === "healthy" || app.health === "disabled")) {
			void connect();
		}
	}, [active, app.health, connect]);

	const handleSendPrompt = useCallback(async () => {
		if (!acpAgent || !activeSessionId || !promptText.trim()) {
			return;
		}

		setIsPrompting(true);
		try {
			await acpAgent.prompt({
				sessionId: activeSessionId,
				prompt: [
					{
						type: "text",
						text: promptText,
					},
				],
			});
			setPromptText("");
		} catch (error) {
			console.error("Failed to send prompt:", error);
		} finally {
			setIsPrompting(false);
		}
	}, [acpAgent, activeSessionId, promptText]);

	if (app.health === "unhealthy") {
		return (
			<div
				className={cn([
					active ? "flex" : "hidden",
					"w-full h-full flex-col items-center justify-center p-4",
				])}
			>
				<h3 className="m-0 font-medium text-content-primary text-base text-center">
					App "{app.display_name}" is unhealthy
				</h3>
				<div className="text-content-secondary text-sm">
					<span className="block text-center">
						Here are some troubleshooting steps you can take:
					</span>
					<ul className="m-0 pt-4 flex flex-col gap-4">
						{app.healthcheck && (
							<li>
								<span className="block font-medium text-content-primary mb-1">
									Verify healthcheck
								</span>
								Try running the following inside your workspace:{" "}
								<code className="font-mono text-content-primary select-all">
									curl -v "{app.healthcheck.url}"
								</code>
							</li>
						)}
						<li>
							<span className="block font-medium text-content-primary mb-1">
								Check logs
							</span>
							See{" "}
							<code className="font-mono text-content-primary select-all">
								/tmp/coder-agent.log
							</code>{" "}
							inside your workspace "{workspace.name}" for more information.
						</li>
					</ul>
				</div>
			</div>
		);
	}

	if (app.health === "initializing") {
		return (
			<div
				className={cn([
					active ? "flex" : "hidden",
					"w-full h-full items-center justify-center",
				])}
			>
				<Spinner loading />
			</div>
		);
	}

	return (
		<div className={cn([active ? "flex" : "hidden", "w-full h-full flex-col"])}>
			{/* Connection Status Bar */}
			{connectionState.status !== "connected" && (
				<div className="bg-surface-tertiary p-2 text-sm text-center border-b border-border">
					{connectionState.status === "connecting" && (
						<span className="text-content-secondary">
							<Spinner loading className="inline-block w-4 h-4 mr-2" />
							Connecting to ACP server...
						</span>
					)}
					{connectionState.status === "error" && (
						<span className="text-red-600">
							Connection error: {connectionState.error}
						</span>
					)}
					{connectionState.status === "disconnected" && (
						<span className="text-content-secondary">Disconnected</span>
					)}
				</div>
			)}

			{/* Messages Area */}
			<div className="flex-1 overflow-y-auto p-4 space-y-4">
				{!activeSessionId && connectionState.status === "connected" && (
					<div className="text-center text-content-secondary text-sm">
						Creating session...
					</div>
				)}

				{mergedMessages.map((message) => (
					<MessageItem key={message.id} notification={message.notification} />
				))}
				<div ref={messagesEndRef} />
			</div>

			{/* Input Area */}
			{activeSessionId && (
				<div className="border-t border-border p-4">
					<div className="flex gap-2">
						<input
							type="text"
							value={promptText}
							onChange={(e) => setPromptText(e.target.value)}
							onKeyDown={(e) => {
								if (e.key === "Enter" && !e.shiftKey && !isPrompting) {
									e.preventDefault();
									void handleSendPrompt();
								}
							}}
							placeholder="Type a message..."
							disabled={isPrompting || connectionState.status !== "connected"}
							className="flex-1 px-3 py-2 border border-border rounded-md focus:outline-none focus:ring-2 focus:ring-highlight text-sm bg-surface-primary"
						/>
						<Button
							onClick={handleSendPrompt}
							disabled={isPrompting || !promptText.trim()}
							size="sm"
						>
							{isPrompting ? <Spinner loading /> : "Send"}
						</Button>
					</div>
				</div>
			)}
		</div>
	);
};

const MessageItem: FC<{ notification: NotificationEvent }> = ({
	notification,
}) => {
	if (notification.type !== "session_notification") {
		return null;
	}

	const update = notification.data.update;

	// User messages
	if (
		update.sessionUpdate === "user_message_chunk" &&
		update.content?.type === "text"
	) {
		return (
			<div className="flex justify-end">
				<div className="bg-highlight text-white px-4 py-2 rounded-lg max-w-[80%]">
					{update.content.text}
				</div>
			</div>
		);
	}

	// Agent messages
	if (
		update.sessionUpdate === "agent_message_chunk" &&
		update.content?.type === "text"
	) {
		return (
			<div className="flex justify-start">
				<div className="bg-surface-secondary px-4 py-2 rounded-lg max-w-[80%]">
					{update.content.text}
				</div>
			</div>
		);
	}

	// Tool calls
	if (update.sessionUpdate === "tool_call") {
		return (
			<div className="flex justify-start">
				<div className="bg-surface-tertiary px-4 py-2 rounded-lg max-w-[80%] text-sm">
					<div className="font-medium text-content-primary mb-1">
						🔧 {update.title || "Tool call"}
					</div>
					<div className="text-content-secondary text-xs">
						Status: {update.status}
					</div>
				</div>
			</div>
		);
	}

	// Agent thoughts (optional)
	if (
		update.sessionUpdate === "agent_thought_chunk" &&
		update.content?.type === "text"
	) {
		return (
			<div className="flex justify-start">
				<div className="bg-surface-tertiary px-4 py-2 rounded-lg max-w-[80%] text-sm italic text-content-secondary">
					{update.content.text}
				</div>
			</div>
		);
	}

	return null;
};

type MergedMessage = {
	id: string;
	notification: NotificationEvent;
};

function mergeMessageChunks(
	notifications: NotificationEvent[],
): MergedMessage[] {
	const result: MergedMessage[] = [];
	let currentChunks: NotificationEvent[] = [];
	let currentType: "agent" | "user" | null = null;

	const flushChunks = () => {
		if (currentChunks.length === 0) return;

		const firstChunk = currentChunks[0];
		const accumulatedText = currentChunks
			.map((n) => {
				if (n.type === "session_notification") {
					const update = n.data.update;
					if (
						(update.sessionUpdate === "agent_message_chunk" ||
							update.sessionUpdate === "user_message_chunk") &&
						"content" in update &&
						update.content?.type === "text"
					) {
						return update.content.text;
					}
				}
				return "";
			})
			.join("");

		// Create a merged notification with accumulated text
		if (firstChunk.type === "session_notification") {
			const update = firstChunk.data.update;
			if (
				(update.sessionUpdate === "agent_message_chunk" ||
					update.sessionUpdate === "user_message_chunk") &&
				"content" in update &&
				update.content?.type === "text"
			) {
				const mergedNotification: NotificationEvent = {
					...firstChunk,
					id: `${firstChunk.id}-merged`,
					data: {
						...firstChunk.data,
						update: {
							...update,
							content: {
								...update.content,
								text: accumulatedText,
							},
						},
					},
				};

				result.push({
					id: mergedNotification.id,
					notification: mergedNotification,
				});
			}
		}

		currentChunks = [];
		currentType = null;
	};

	for (const notification of notifications) {
		if (notification.type !== "session_notification") {
			flushChunks();
			result.push({ id: notification.id, notification });
			continue;
		}

		const update = notification.data.update;

		// Check if this is a message chunk
		if (
			update.sessionUpdate === "agent_message_chunk" &&
			update.content?.type === "text"
		) {
			if (currentType !== "agent") {
				flushChunks();
				currentType = "agent";
			}
			currentChunks.push(notification);
		} else if (
			update.sessionUpdate === "user_message_chunk" &&
			update.content?.type === "text"
		) {
			if (currentType !== "user") {
				flushChunks();
				currentType = "user";
			}
			currentChunks.push(notification);
		} else {
			// Different notification type - flush accumulated chunks
			flushChunks();
			result.push({ id: notification.id, notification });
		}
	}

	// Flush any remaining chunks
	flushChunks();

	return result;
}

function constructWsUrl(httpUrl: string): string {
	try {
		// Handle relative URLs by providing a base
		const url = new URL(httpUrl, window.location.origin);
		url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
		return url.toString();
	} catch (error) {
		console.error("Failed to construct WebSocket URL:", error);
		return httpUrl;
	}
}
