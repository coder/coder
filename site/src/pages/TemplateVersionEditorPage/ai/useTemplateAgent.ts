import { createAnthropic } from "@ai-sdk/anthropic";
import { createMCPClient } from "@ai-sdk/mcp";
import { createOpenAI } from "@ai-sdk/openai";
import {
	createAgentUIStream,
	getToolName,
	isReasoningUIPart,
	isToolUIPart,
	readUIMessageStream,
	stepCountIs,
	ToolLoopAgent,
	type ToolSet,
	type UIMessage,
} from "ai";
import { API } from "api/api";
import type { AIBridgeProvider, AIModelConfig } from "api/queries/aiBridge";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { FileTree } from "utils/filetree";
import type {
	BuildOutput,
	BuildResult,
	PublishRequestData,
	PublishRequestOptions,
	PublishResult,
} from "./tools";
import { createTemplateAgentTools } from "./tools";
import type { AgentStatus, PendingToolCall } from "./types";

/**
 * Read the runtime CSRF token from the Axios instance's default
 * headers. This is the correct token in both development (hardcoded)
 * and production (derived from the page's meta tag at startup).
 * Using API.getCsrfToken() would always return the hardcoded
 * development-only value.
 */
function getRuntimeCsrfToken(): string {
	const headers = API.getAxiosInstance().defaults.headers.common;
	const token = headers["X-CSRF-TOKEN"];
	if (typeof token === "string") {
		return token;
	}
	return "";
}

const openAIProvider = createOpenAI({
	baseURL: "/api/v2/aibridge/openai/v1",
	apiKey: "coder",
	headers: {
		"X-CSRF-TOKEN": getRuntimeCsrfToken(),
		// Override the SDK's Authorization header so the AI bridge
		// authenticates via the browser's session cookie instead of
		// this placeholder key.
		Authorization: "",
	},
});

const anthropicProvider = createAnthropic({
	baseURL: "/api/v2/aibridge/anthropic/v1",
	apiKey: "coder",
	headers: {
		"X-CSRF-TOKEN": getRuntimeCsrfToken(),
		// Override the SDK's x-api-key header so the AI bridge
		// authenticates via the browser's session cookie instead of
		// this placeholder key.
		"x-api-key": "",
	},
});

const anthropicModelPrefix = "anthropic/";

const resolveProviderModel = (provider: AIBridgeProvider, modelID: string) => {
	if (modelID.length === 0) {
		throw new Error("Model ID cannot be empty.");
	}

	if (provider === "anthropic") {
		const anthropicModelID = modelID.startsWith(anthropicModelPrefix)
			? modelID.slice(anthropicModelPrefix.length)
			: modelID;
		if (anthropicModelID.length === 0) {
			throw new Error("Anthropic model ID cannot be empty.");
		}
		return anthropicProvider(anthropicModelID);
	}

	return openAIProvider(modelID);
};

type MCPTools = Awaited<
	ReturnType<Awaited<ReturnType<typeof createMCPClient>>["tools"]>
>;

const MAX_STEPS = 20;

const SYSTEM_PROMPT = `You are a Terraform template editing assistant for Coder.
You help users modify Coder workspace templates (Terraform HCL files).

Rules:
- Always use listFiles first to see the template structure.
- Always use readFile before editing a file.
- Use editFile for targeted changes — provide enough context in oldContent
  to uniquely identify the edit location.
- Keep HCL syntax valid. Use proper Terraform formatting conventions.
- Explain what you're changing and why before making edits.
- After making changes, use buildTemplate to validate them.
- If a build fails, use getBuildLogs to understand the error and fix it.
- When the user asks about build errors, use getBuildLogs to read the logs.
- When the user asks you to publish (or says "build and publish"),
  call publishTemplate directly with sensible defaults — do not ask
  follow-up questions about version name or changelog message.
  The tool requires user approval, so they will get a chance to
  review before it executes.
- For publishTemplate: omit name to keep the current version name.
  Generate a short changelog message from the changes you made.`;

const createTemplateAgent = (
	modelConfig: AIModelConfig,
	getFileTree: () => FileTree,
	setFileTree: (updater: (prev: FileTree) => FileTree) => void,
	hasBuiltInCurrentRunRef: { current: boolean },
	callbacks: {
		onFileEdited?: (path: string) => void;
		onFileDeleted?: (path: string) => void;
		onBuildRequested?: () => Promise<void>;
		waitForBuildComplete?: () => Promise<BuildResult>;
		getBuildOutput?: () => BuildOutput | undefined;
		onPublishRequested?: (
			data: PublishRequestData,
			options?: PublishRequestOptions,
		) => Promise<PublishResult>;
	},
	externalTools: MCPTools,
) => {
	const providerOptions: NonNullable<
		ConstructorParameters<typeof ToolLoopAgent>[0]["providerOptions"]
	> = {};
	if (
		modelConfig.model.provider === "openai" &&
		modelConfig.reasoningEffort !== undefined
	) {
		providerOptions.openai = {
			reasoningEffort: modelConfig.reasoningEffort,
			// Request reasoning summaries so we can display them
			// in the chat UI. Without this, OpenAI reasoning
			// models think internally but don't expose traces.
			reasoningSummary: "auto",
		};
	}
	if (modelConfig.model.provider === "anthropic" && modelConfig.thinking) {
		providerOptions.anthropic = {
			thinking: modelConfig.thinking,
			...(modelConfig.anthropicEffort !== undefined && {
				effort: modelConfig.anthropicEffort,
			}),
		};
	}

	const localTools = createTemplateAgentTools(
		getFileTree,
		setFileTree,
		hasBuiltInCurrentRunRef,
		callbacks,
	);

	return new ToolLoopAgent({
		model: resolveProviderModel(
			modelConfig.model.provider,
			modelConfig.model.id,
		),
		instructions: SYSTEM_PROMPT,
		tools: { ...localTools, ...(externalTools as ToolSet) },
		stopWhen: stepCountIs(MAX_STEPS),
		providerOptions,
	});
};

interface UseTemplateAgentOptions {
	getFileTree: () => FileTree;
	setFileTree: (updater: (prev: FileTree) => FileTree) => void;
	modelConfig: AIModelConfig;
	/** Called after a file is created or edited so the editor can navigate to it. */
	onFileEdited?: (path: string) => void;
	/** Called after a file is deleted so the editor can clear the active path if needed. */
	onFileDeleted?: (path: string) => void;
	/** Triggers a template build (uploads files, creates version). */
	onBuildRequested?: () => Promise<void>;
	/** Returns a promise that resolves when the current build reaches a terminal state. */
	waitForBuildComplete?: () => Promise<BuildResult>;
	/** Returns the current build output snapshot, or undefined if no build has run. */
	getBuildOutput?: () => BuildOutput | undefined;
	/** Publishes the current template version. Returns success/error. */
	onPublishRequested?: (
		data: PublishRequestData,
		options?: PublishRequestOptions,
	) => Promise<PublishResult>;
}

export interface DisplayToolCall {
	toolCallId: string;
	toolName: string;
	args: Record<string, unknown>;
	result?: unknown;
	state: "pending" | "result";
}

export interface DisplayReasoning {
	text: string;
	isStreaming: boolean;
}

export interface DisplayMessage {
	id: string;
	role: "user" | "assistant";
	content: string;
	toolCalls: DisplayToolCall[];
	reasoning: DisplayReasoning[];
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

const toToolArgs = (input: unknown): Record<string, unknown> =>
	isRecord(input) ? input : {};

const cloneMessage = <T>(value: T): T => {
	if (typeof globalThis.structuredClone === "function") {
		return globalThis.structuredClone(value);
	}
	return JSON.parse(JSON.stringify(value)) as T;
};

const upsertMessage = (
	messages: UIMessage[],
	message: UIMessage,
): UIMessage[] => {
	const index = messages.findIndex((existing) => existing.id === message.id);
	if (index === -1) {
		return [...messages, message];
	}
	const next = [...messages];
	next[index] = message;
	return next;
};

const mapToolStateToDisplay = (
	part: Parameters<typeof getToolName>[0],
): DisplayToolCall => {
	const toolName = getToolName(part);
	const args = toToolArgs(part.input);

	switch (part.state) {
		case "output-available":
			return {
				toolCallId: part.toolCallId,
				toolName,
				args,
				result: part.output,
				state: "result",
			};
		case "output-error":
			return {
				toolCallId: part.toolCallId,
				toolName,
				args,
				result: { error: part.errorText },
				state: "result",
			};
		case "output-denied":
			return {
				toolCallId: part.toolCallId,
				toolName,
				args,
				result: {
					error: part.approval.reason ?? "User rejected this action.",
				},
				state: "result",
			};
		default:
			return {
				toolCallId: part.toolCallId,
				toolName,
				args,
				state: "pending",
			};
	}
};

const toDisplayMessages = (uiMessages: UIMessage[]): DisplayMessage[] => {
	const result: DisplayMessage[] = [];

	for (const message of uiMessages) {
		if (message.role !== "user" && message.role !== "assistant") {
			continue;
		}

		if (message.role === "user") {
			const content = message.parts
				.filter(
					(part): part is { type: "text"; text: string } =>
						part.type === "text",
				)
				.map((part) => part.text)
				.join("");
			result.push({
				id: message.id,
				role: "user",
				content,
				toolCalls: [],
				reasoning: [],
			});
			continue;
		}

		// Split assistant messages into chronological segments.
		// A new segment starts when a text part follows a tool
		// part, preserving the natural conversation flow:
		// reasoning → text → tool calls → reasoning → text → tool calls.
		let segmentIndex = 0;
		let currentText = "";
		let currentToolCalls: DisplayToolCall[] = [];
		let currentReasoning: DisplayReasoning[] = [];
		let lastPartWasTool = false;

		for (const part of message.parts) {
			if (part.type === "text") {
				// Flush the current segment when text follows
				// tool calls — this starts a new visual block.
				if (lastPartWasTool && currentToolCalls.length > 0) {
					result.push({
						id: `${message.id}-${segmentIndex++}`,
						role: "assistant",
						content: currentText,
						toolCalls: currentToolCalls,
						reasoning: currentReasoning,
					});
					currentText = "";
					currentToolCalls = [];
					currentReasoning = [];
				}
				currentText += part.text;
				lastPartWasTool = false;
			} else if (isReasoningUIPart(part)) {
				// Flush when reasoning follows tool calls, just like
				// text does — reasoning belongs to the next segment.
				if (lastPartWasTool && currentToolCalls.length > 0) {
					result.push({
						id: `${message.id}-${segmentIndex++}`,
						role: "assistant",
						content: currentText,
						toolCalls: currentToolCalls,
						reasoning: currentReasoning,
					});
					currentText = "";
					currentToolCalls = [];
					currentReasoning = [];
				}
				currentReasoning.push({
					text: part.text,
					isStreaming: part.state === "streaming",
				});
				lastPartWasTool = false;
			} else if (isToolUIPart(part)) {
				currentToolCalls.push(mapToolStateToDisplay(part));
				lastPartWasTool = true;
			}
		}

		// Flush the final segment. Use the original message ID
		// when only one segment was produced so that approval
		// lookups still match by ID.
		if (
			currentText.length > 0 ||
			currentToolCalls.length > 0 ||
			currentReasoning.length > 0
		) {
			result.push({
				id: segmentIndex > 0 ? `${message.id}-${segmentIndex}` : message.id,
				role: "assistant",
				content: currentText,
				toolCalls: currentToolCalls,
				reasoning: currentReasoning,
			});
		}
	}

	return result;
};

const collectPendingApprovals = (
	uiMessages: UIMessage[],
): PendingToolCall[] => {
	const pending: PendingToolCall[] = [];

	for (const message of uiMessages) {
		if (message.role !== "assistant") {
			continue;
		}

		for (const part of message.parts) {
			if (!isToolUIPart(part) || part.state !== "approval-requested") {
				continue;
			}

			const toolName = getToolName(part);
			if (
				toolName !== "editFile" &&
				toolName !== "deleteFile" &&
				toolName !== "buildTemplate" &&
				toolName !== "publishTemplate"
			) {
				continue;
			}

			pending.push({
				approvalId: part.approval.id,
				toolCallId: part.toolCallId,
				toolName,
				args: toToolArgs(part.input),
			});
		}
	}

	return pending;
};

const applyApprovalResponse = (
	messages: UIMessage[],
	pending: PendingToolCall,
	approved: boolean,
	reason?: string,
): { nextMessages: UIMessage[]; updated: boolean } => {
	let updated = false;

	const nextMessages = messages.map((message) => {
		if (message.role !== "assistant") {
			return message;
		}

		let messageUpdated = false;
		const nextParts = message.parts.map((part) => {
			if (!isToolUIPart(part)) {
				return part;
			}
			if (
				part.toolCallId !== pending.toolCallId ||
				part.state !== "approval-requested"
			) {
				return part;
			}
			if (
				getToolName(part) !== pending.toolName ||
				part.approval.id !== pending.approvalId
			) {
				return part;
			}

			updated = true;
			messageUpdated = true;

			const approval = reason
				? { id: pending.approvalId, approved, reason }
				: { id: pending.approvalId, approved };
			return {
				...part,
				state: "approval-responded",
				approval,
			} as UIMessage["parts"][number];
		});

		return messageUpdated ? { ...message, parts: nextParts } : message;
	});

	return { nextMessages, updated };
};

export const useTemplateAgent = ({
	getFileTree,
	setFileTree,
	modelConfig,
	onFileEdited,
	onFileDeleted,
	onBuildRequested,
	waitForBuildComplete,
	getBuildOutput,
	onPublishRequested,
}: UseTemplateAgentOptions) => {
	const [uiMessages, setUIMessages] = useState<UIMessage[]>([]);
	const [status, setStatus] = useState<AgentStatus>("idle");

	const uiMessagesRef = useRef<UIMessage[]>([]);
	const messageCounter = useRef(0);
	const abortRef = useRef<AbortController | null>(null);

	const mcpClientRef =
		useRef<Awaited<ReturnType<typeof createMCPClient>> | null>(null);
	const mcpToolsRef = useRef<MCPTools>({});

	// Tracks whether a successful build happened for the current chat
	// session so publish can skip the dirty-file check after approval
	// pauses resume the stream.
	const hasBuiltInCurrentRunRef = useRef(false);

	// Ref wrappers for tool callbacks that may change between steps of a
	// multi-step stream (e.g., dirty/build-status changes after an edit).
	// Dereferencing through the ref ensures each tool invocation reads
	// the latest editor/build state.
	const toolCallbacksRef = useRef({
		onFileEdited,
		onFileDeleted,
		onBuildRequested,
		waitForBuildComplete,
		getBuildOutput,
		onPublishRequested,
	});
	toolCallbacksRef.current = {
		onFileEdited,
		onFileDeleted,
		onBuildRequested,
		waitForBuildComplete,
		getBuildOutput,
		onPublishRequested,
	};

	useEffect(() => {
		let cancelled = false;

		const initMCP = async () => {
			try {
				const client = await createMCPClient({
					transport: {
						type: "http",
						url: "https://dev.registry.coder.com/mcp",
					},
				});
				if (cancelled) {
					await client.close();
					return;
				}

				mcpClientRef.current = client;
				mcpToolsRef.current = await client.tools();
			} catch (error) {
				// Best-effort integration: keep local tools available if MCP
				// initialization fails.
				console.warn("Failed to initialize MCP client:", error);
			}
		};

		void initMCP();

		return () => {
			cancelled = true;
			if (mcpClientRef.current) {
				void mcpClientRef.current.close();
				mcpClientRef.current = null;
			}
			mcpToolsRef.current = {};
		};
	}, []);

	const setConversationMessages = useCallback((next: UIMessage[]) => {
		uiMessagesRef.current = next;
		setUIMessages(next);
	}, []);

	const runStream = useCallback(
		async (conversation: UIMessage[]) => {
			abortRef.current?.abort();
			const abortController = new AbortController();
			abortRef.current = abortController;
			setStatus("streaming");

			const finishRun = (nextStatus: AgentStatus) => {
				if (abortRef.current !== abortController) {
					return;
				}
				abortRef.current = null;
				setStatus(nextStatus);
			};

			const agent = createTemplateAgent(
				modelConfig,
				getFileTree,
				setFileTree,
				hasBuiltInCurrentRunRef,
				{
					onFileEdited: (path) => toolCallbacksRef.current.onFileEdited?.(path),
					onFileDeleted: (path) =>
						toolCallbacksRef.current.onFileDeleted?.(path),
					onBuildRequested: toolCallbacksRef.current.onBuildRequested
						? () =>
								toolCallbacksRef.current.onBuildRequested?.() ??
								Promise.resolve()
						: undefined,
					waitForBuildComplete: toolCallbacksRef.current.waitForBuildComplete
						? () =>
								toolCallbacksRef.current.waitForBuildComplete?.() ??
								Promise.resolve<BuildResult>({
									status: "failed",
									error: "Build tools are not available.",
									logs: "",
								})
						: undefined,
					getBuildOutput: toolCallbacksRef.current.getBuildOutput
						? () => toolCallbacksRef.current.getBuildOutput?.()
						: undefined,
					onPublishRequested: toolCallbacksRef.current.onPublishRequested
						? (data, options) =>
								toolCallbacksRef.current.onPublishRequested?.(data, options) ??
								Promise.resolve<PublishResult>({
									success: false,
									error: "Publish is not available.",
								})
						: undefined,
				},
				mcpToolsRef.current,
			);

			let stream: Awaited<ReturnType<typeof createAgentUIStream>>;
			try {
				stream = await createAgentUIStream({
					agent,
					uiMessages: conversation,
					abortSignal: abortController.signal,
				});
			} catch {
				if (!abortController.signal.aborted) {
					finishRun("error");
				} else {
					finishRun("idle");
				}
				return;
			}

			let nextConversation = conversation;
			const lastMessage = conversation[conversation.length - 1];
			const initialAssistantMessage =
				lastMessage?.role === "assistant"
					? cloneMessage(lastMessage)
					: {
							id: `assistant-${++messageCounter.current}`,
							role: "assistant" as const,
							parts: [],
						};

			try {
				for await (const message of readUIMessageStream({
					stream,
					message: initialAssistantMessage,
				})) {
					if (abortController.signal.aborted) {
						break;
					}

					nextConversation = upsertMessage(uiMessagesRef.current, message);
					setConversationMessages(nextConversation);
				}
			} catch {
				if (!abortController.signal.aborted) {
					finishRun("error");
				} else {
					finishRun("idle");
				}
				return;
			}

			if (abortController.signal.aborted) {
				finishRun("idle");
				return;
			}

			const pending = collectPendingApprovals(nextConversation);
			finishRun(pending.length > 0 ? "awaiting_approval" : "idle");
		},
		[getFileTree, modelConfig, setConversationMessages, setFileTree],
	);

	const send = useCallback(
		(text: string) => {
			if (status === "streaming") {
				return;
			}
			// Don't allow new messages while approvals are pending.
			if (status === "awaiting_approval") {
				return;
			}
			if (abortRef.current) {
				return;
			}
			if (status === "error") {
				setStatus("idle");
			}

			const trimmed = text.trim();
			if (!trimmed) {
				return;
			}

			if (uiMessagesRef.current.length === 0) {
				hasBuiltInCurrentRunRef.current = false;
			}

			const userMessage: UIMessage = {
				id: `msg-${++messageCounter.current}`,
				role: "user",
				parts: [{ type: "text", text: trimmed }],
			};
			const nextConversation = [...uiMessagesRef.current, userMessage];
			setConversationMessages(nextConversation);

			void runStream(nextConversation);
		},
		[runStream, setConversationMessages, status],
	);

	const pendingApprovals = useMemo(
		() => collectPendingApprovals(uiMessages),
		[uiMessages],
	);

	const approve = useCallback(() => {
		if (status !== "awaiting_approval") {
			return;
		}

		const current = pendingApprovals[0];
		if (!current) {
			return;
		}

		const { nextMessages, updated } = applyApprovalResponse(
			uiMessagesRef.current,
			current,
			true,
		);
		if (!updated) {
			setStatus("error");
			return;
		}

		setConversationMessages(nextMessages);
		const remaining = collectPendingApprovals(nextMessages);
		if (remaining.length > 0) {
			setStatus("awaiting_approval");
			return;
		}

		void runStream(nextMessages);
	}, [pendingApprovals, runStream, setConversationMessages, status]);

	const reject = useCallback(() => {
		if (status !== "awaiting_approval") {
			return;
		}

		const current = pendingApprovals[0];
		if (!current) {
			return;
		}

		const { nextMessages, updated } = applyApprovalResponse(
			uiMessagesRef.current,
			current,
			false,
			"User rejected this action.",
		);
		if (!updated) {
			setStatus("error");
			return;
		}

		setConversationMessages(nextMessages);
		const remaining = collectPendingApprovals(nextMessages);
		if (remaining.length > 0) {
			setStatus("awaiting_approval");
			return;
		}

		void runStream(nextMessages);
	}, [pendingApprovals, runStream, setConversationMessages, status]);

	const stop = useCallback(() => {
		abortRef.current?.abort();
		abortRef.current = null;
		const pending = collectPendingApprovals(uiMessagesRef.current);
		setStatus(pending.length > 0 ? "awaiting_approval" : "idle");
	}, []);

	const reset = useCallback(() => {
		abortRef.current?.abort();
		abortRef.current = null;
		hasBuiltInCurrentRunRef.current = false;
		messageCounter.current = 0;
		setConversationMessages([]);
		setStatus("idle");
	}, [setConversationMessages]);

	const resetBuildState = useCallback(() => {
		hasBuiltInCurrentRunRef.current = false;
	}, []);

	const messages = useMemo(() => toDisplayMessages(uiMessages), [uiMessages]);
	const pendingApproval =
		pendingApprovals.length > 0 ? pendingApprovals[0] : null;

	return {
		messages,
		isStreaming: status === "streaming",
		status,
		pendingApproval,
		send,
		approve,
		reject,
		stop,
		reset,
		resetBuildState,
	};
};

export type TemplateAgentState = ReturnType<typeof useTemplateAgent>;
