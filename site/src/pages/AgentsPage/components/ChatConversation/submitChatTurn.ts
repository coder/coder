import { toast } from "sonner";
import type {
	ChatPlanModeOrClear,
	CreateChatMessageRequestWithClearablePlanMode,
} from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import { buildOptimisticEditedMessage } from "#/api/queries/chatMessageEdits";
import type * as TypesGen from "#/api/typesGenerated";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import { BuiltInCommandPendingError } from "../../hooks/useConversationEditingState";
import {
	buildAttachmentMediaTypes,
	buildChatInputContent,
	type ChatComposerContentPart,
} from "../../utils/chatInputContent";
import { isUnavailableHistoricalModelID } from "../../utils/modelOptions";
import {
	CHAT_SLASH_COMMANDS,
	CLEAR_SLASH_COMMAND,
	COMPACT_SLASH_COMMAND,
	chatSlashCommandTriggerText,
	resolveChatSlashCommandAvailability,
} from "../../utils/slashCommands";
import type { PendingAttachment } from "../ChatPageContent";
import {
	buildInactiveChatQueueReconciliation,
	reconcilePromotedQueueHead,
	restoreOptimisticRequestSnapshot,
	settlePromotedQueueHead,
	submitEdit,
	waitForPendingChatSettingsSyncs,
} from "./chatQueueReconciliation";
import type { ChatStore } from "./chatStore";

/** @internal Exported for testing. */
export const lastModelConfigIDStorageKey = "agents.last-model-config-id";

const clearChatPlanMode = "" satisfies ChatPlanModeOrClear;

type PlanModeSwitch = TypesGen.ChatPlanMode | "clear";

export type SubmitChatTurnParams = {
	message: string;
	attachments?: readonly PendingAttachment[];
	editedMessageID?: number;
	composerParts?: readonly ChatComposerContentPart[];
	planModeSwitch?: PlanModeSwitch;
	isSubmissionPending: boolean;
	hasModelOptions: boolean;
	pendingPlanModeSyncRef: { current: Promise<unknown> | null };
	pendingWorkspaceSyncRef: { current: Promise<unknown> | null };
	isEditReasoningEffortDirtyRef: { current: boolean };
	personalSkills: readonly { name: string }[] | undefined;
	workspaceSkills: readonly { name: string }[] | undefined;
	compact: () => Promise<unknown>;
	clearChatContext: () => Promise<unknown>;
	store: ChatStore;
	agentId: string;
	clearChatErrorReason: (chatId: string) => void;
	acceptServerChatStatus: () => void;
	chatMessages: readonly TypesGen.ChatMessage[] | undefined;
	effectiveSelectedModel: string;
	modelOptions: readonly ModelSelectorOption[];
	effectiveReasoningEffort: string | undefined;
	mcpServerIds: readonly string[];
	editMessage: (args: {
		messageId: number;
		optimisticMessage?: TypesGen.ChatMessage;
		req: TypesGen.EditChatMessageRequest;
	}) => Promise<unknown>;
	sendMessage: (
		req: CreateChatMessageRequestWithClearablePlanMode,
	) => Promise<TypesGen.CreateChatMessageResponse>;
	onRequestError: (error: unknown) => void;
	invalidateChat: (chatId: string) => void;
	scrollToEnd: (options: { behavior: "smooth" }) => void;
	upsertCacheMessages: (messages: readonly TypesGen.ChatMessage[]) => void;
	getCacheQueuedMessages: () =>
		| readonly TypesGen.ChatQueuedMessage[]
		| undefined;
	setCacheQueuedMessages: (
		messages: readonly TypesGen.ChatQueuedMessage[],
	) => void;
	fetchQueueConvergence: (
		chatId: string,
	) => Promise<TypesGen.ChatMessagesResponse>;
	setCachedChatPlanMode: (
		chatId: string,
		planMode?: TypesGen.ChatPlanMode,
	) => void;
};

/** @internal Exported for testing. */
export const resolveEditModelConfigID = ({
	pickerModelConfigID,
	originalModelConfigID,
	modelOptions,
}: {
	pickerModelConfigID: string | undefined;
	originalModelConfigID: string | undefined;
	modelOptions: readonly ModelSelectorOption[];
}): string | undefined => {
	if (!pickerModelConfigID) {
		return undefined;
	}
	const originalIsSelectable =
		originalModelConfigID !== undefined &&
		modelOptions.some((option) => option.id === originalModelConfigID);
	const originalIsUnavailable = isUnavailableHistoricalModelID(
		originalModelConfigID,
		modelOptions,
	);
	// Use the picker fallback for an unavailable historical model.
	// Override a selectable model only after the user changes it.
	// Omit blank and nil references so the backend preserves the original.
	if (
		originalIsUnavailable ||
		(originalIsSelectable && pickerModelConfigID !== originalModelConfigID)
	) {
		return pickerModelConfigID;
	}
	return undefined;
};

const persistLastModelConfigID = (modelConfigID: string): void => {
	localStorage.setItem(lastModelConfigIDStorageKey, modelConfigID);
};

const findBuiltInChatCommand = (
	content: readonly TypesGen.ChatInputPart[],
	editedMessageID: number | undefined,
): (typeof CHAT_SLASH_COMMANDS)[number] | undefined => {
	// Built-ins only intercept new, text-only sends. A personal or workspace
	// skill with the same name takes precedence at availability resolution.
	if (editedMessageID !== undefined || content.length !== 1) {
		return undefined;
	}
	const [part] = content;
	if (part.type !== "text") {
		return undefined;
	}
	const trigger = part.text?.trim();
	return CHAT_SLASH_COMMANDS.find(
		(command) => trigger === chatSlashCommandTriggerText(command),
	);
};

const runBuiltInChatCommand = async ({
	command,
	store,
	agentId,
	clearChatErrorReason,
	compact,
	clearChatContext,
}: {
	command: (typeof CHAT_SLASH_COMMANDS)[number];
	store: ChatStore;
	agentId: string;
	clearChatErrorReason: (chatId: string) => void;
	compact: () => Promise<unknown>;
	clearChatContext: () => Promise<unknown>;
}): Promise<void> => {
	switch (command.name) {
		case COMPACT_SLASH_COMMAND.name: {
			// Set running before awaiting so the worker's streamed waiting status
			// cannot be overwritten if it arrives before the POST resolves.
			const previousSnapshot = store.getSnapshot();
			clearChatErrorReason(agentId);
			store.clearStreamError();
			store.clearStreamState();
			store.setChatStatus("running");
			try {
				await compact();
			} catch (error) {
				restoreOptimisticRequestSnapshot(store, previousSnapshot);
				toast.error(getErrorMessage(error, "Failed to compact chat."));
				throw error;
			}
			return;
		}
		case CLEAR_SLASH_COMMAND.name:
			try {
				await clearChatContext();
			} catch (error) {
				toast.error(getErrorMessage(error, "Failed to clear chat."));
				throw error;
			}
	}
};

const applyQueuedSendReconciliation = ({
	store,
	agentId,
	isActiveChat,
	insertedMessages,
	queuedMessagesBeforeSend,
	queueHeadIDBeforeSend,
	statusVersionBeforeSend,
	queuedTail,
	getCacheQueuedMessages,
	setCacheQueuedMessages,
	fetchQueueConvergence,
}: {
	store: ChatStore;
	agentId: string;
	isActiveChat: boolean;
	insertedMessages: readonly TypesGen.ChatMessage[];
	queuedMessagesBeforeSend: readonly TypesGen.ChatQueuedMessage[];
	queueHeadIDBeforeSend: number | undefined;
	statusVersionBeforeSend: number;
	queuedTail: TypesGen.ChatQueuedMessage | undefined;
	getCacheQueuedMessages: SubmitChatTurnParams["getCacheQueuedMessages"];
	setCacheQueuedMessages: SubmitChatTurnParams["setCacheQueuedMessages"];
	fetchQueueConvergence: SubmitChatTurnParams["fetchQueueConvergence"];
}): void => {
	const reconciledQueue = isActiveChat
		? reconcilePromotedQueueHead(
				store,
				insertedMessages,
				queueHeadIDBeforeSend,
				queuedTail,
			)
		: buildInactiveChatQueueReconciliation(
				getCacheQueuedMessages(),
				queuedMessagesBeforeSend,
				insertedMessages,
				queueHeadIDBeforeSend,
				queuedTail,
			);
	if (!reconciledQueue) {
		return;
	}
	setCacheQueuedMessages(reconciledQueue);
	// A promoted head starts a turn, but any server status received during
	// the request is newer and must win.
	if (
		isActiveChat &&
		store.getServerChatStatusVersion() === statusVersionBeforeSend
	) {
		store.clearStreamState();
		store.setChatStatus("running");
	}
	if (isActiveChat && queueHeadIDBeforeSend !== undefined) {
		void settlePromotedQueueHead(
			store,
			agentId,
			queueHeadIDBeforeSend,
			fetchQueueConvergence,
		).then((settled) => {
			if (settled) {
				setCacheQueuedMessages(settled);
			}
		});
	}
};

export async function submitChatTurn(
	params: SubmitChatTurnParams,
): Promise<void> {
	const {
		message,
		attachments,
		editedMessageID,
		composerParts,
		planModeSwitch,
		isSubmissionPending,
		hasModelOptions,
		pendingPlanModeSyncRef,
		pendingWorkspaceSyncRef,
		personalSkills,
		workspaceSkills,
		compact,
		clearChatContext,
		store,
		agentId,
		clearChatErrorReason,
		acceptServerChatStatus,
		chatMessages,
		effectiveSelectedModel,
		modelOptions,
		effectiveReasoningEffort,
		isEditReasoningEffortDirtyRef,
		mcpServerIds,
		editMessage,
		sendMessage,
		onRequestError,
		invalidateChat,
		scrollToEnd,
		upsertCacheMessages,
		getCacheQueuedMessages,
		setCacheQueuedMessages,
		fetchQueueConvergence,
		setCachedChatPlanMode,
	} = params;

	const { content, hasContent } = buildChatInputContent({
		message,
		attachments,
		composerParts,
	});
	if (!hasContent || isSubmissionPending || !hasModelOptions) {
		return;
	}
	// Wait for chat-setting mutations to settle before sending so the
	// message observes the workspace and plan-mode choices the user just made.
	await waitForPendingChatSettingsSyncs([
		pendingPlanModeSyncRef.current,
		pendingWorkspaceSyncRef.current,
	]);

	const builtInCommand = findBuiltInChatCommand(content, editedMessageID);
	const builtInCommandResolution = builtInCommand
		? resolveChatSlashCommandAvailability(
				builtInCommand,
				personalSkills,
				workspaceSkills,
			)
		: undefined;
	if (builtInCommandResolution === "pending" && builtInCommand) {
		const triggerText = chatSlashCommandTriggerText(builtInCommand);
		toast.info(
			`Checking whether ${triggerText} is available. Try again in a moment.`,
		);
		throw new BuiltInCommandPendingError();
	}
	if (builtInCommandResolution === "available" && builtInCommand) {
		await runBuiltInChatCommand({
			command: builtInCommand,
			store,
			agentId,
			clearChatErrorReason,
			compact,
			clearChatContext,
		});
		return;
	}

	if (editedMessageID !== undefined) {
		const originalEditedMessage = chatMessages?.find(
			(existingMessage) => existingMessage.id === editedMessageID,
		);
		const editSelectedModelConfigID = resolveEditModelConfigID({
			pickerModelConfigID: effectiveSelectedModel || undefined,
			originalModelConfigID: originalEditedMessage?.model_config_id,
			modelOptions,
		});
		const request: TypesGen.EditChatMessageRequest = {
			content,
			model_config_id: editSelectedModelConfigID,
			// Omit so the backend preserves the original effort.
			reasoning_effort: isEditReasoningEffortDirtyRef.current
				? effectiveReasoningEffort
				: undefined,
			mcp_server_ids: [...mcpServerIds],
		};
		const optimisticMessage = originalEditedMessage
			? buildOptimisticEditedMessage({
					requestContent: request.content,
					originalMessage: originalEditedMessage,
					attachmentMediaTypes: buildAttachmentMediaTypes(attachments),
				})
			: undefined;
		const previousSnapshot = store.getSnapshot();
		clearChatErrorReason(agentId);
		store.clearStreamError();
		store.batch(() => {
			store.setQueuedMessages([]);
			store.setChatStatus("running");
			store.clearStreamState();
		});
		await submitEdit({
			editMessage,
			editArgs: {
				messageId: editedMessageID,
				optimisticMessage,
				req: request,
			},
			onError: (error) => {
				restoreOptimisticRequestSnapshot(store, previousSnapshot);
				onRequestError(error);
				// Hook dispatch failures can park an idle chat in error before
				// returning the request error.
				acceptServerChatStatus();
				invalidateChat(agentId);
			},
		});
		scrollToEnd({ behavior: "smooth" });
		if (editSelectedModelConfigID) {
			persistLastModelConfigID(editSelectedModelConfigID);
		}
		return;
	}

	const selectedModelConfigID = effectiveSelectedModel || undefined;
	const request: CreateChatMessageRequestWithClearablePlanMode = {
		content,
		model_config_id: selectedModelConfigID,
		reasoning_effort: effectiveReasoningEffort,
		mcp_server_ids: [...mcpServerIds],
		...(planModeSwitch !== undefined
			? {
					plan_mode:
						planModeSwitch === "clear" ? clearChatPlanMode : planModeSwitch,
				}
			: {}),
	};
	clearChatErrorReason(agentId);
	store.clearStreamError();

	// An errored-chat send may promote the queue head that existed when
	// the request began.
	const queuedMessagesBeforeSend = store.getSnapshot().queuedMessages;
	const queueHeadIDBeforeSend = queuedMessagesBeforeSend[0]?.id;
	const statusVersionBeforeSend = store.getServerChatStatusVersion();

	// Don't clear stream state before the POST completes. For queued sends
	// the WebSocket status events handle clearing; for non-queued sends we
	// clear explicitly below. Clearing eagerly causes a visible cutoff.
	let response: TypesGen.CreateChatMessageResponse;
	try {
		response = await sendMessage(request);
	} catch (error) {
		onRequestError(error);
		acceptServerChatStatus();
		invalidateChat(agentId);
		throw error;
	}
	const isActiveChat = store.getActiveChatID() === agentId;
	// Waiting for the WebSocket on non-queued sends leaves stale stream
	// state visible.
	if (!response.queued && isActiveChat) {
		store.clearStreamState();
		// The server accepted the message (not queued), so it will start
		// processing. The WebSocket status:running event no-ops via the
		// setChatStatus guard. If the server transitions to error/pending
		// instead, the WebSocket event overrides this optimistic value.
		store.setChatStatus("running");
	}
	// Upsert the full batch because a queued send can insert a promoted
	// head below the highest cached ID, which a reconnect would skip.
	const insertedMessages =
		response.messages ?? (response.message ? [response.message] : []);
	if (insertedMessages.length > 0) {
		upsertCacheMessages(insertedMessages);
		if (isActiveChat) {
			store.upsertDurableMessages(insertedMessages);
		}
		if (response.queued) {
			applyQueuedSendReconciliation({
				store,
				agentId,
				isActiveChat,
				insertedMessages,
				queuedMessagesBeforeSend,
				queueHeadIDBeforeSend,
				statusVersionBeforeSend,
				queuedTail: response.queued_message,
				getCacheQueuedMessages,
				setCacheQueuedMessages,
				fetchQueueConvergence,
			});
		}
	}
	if (selectedModelConfigID) {
		persistLastModelConfigID(selectedModelConfigID);
	} else {
		localStorage.removeItem(lastModelConfigIDStorageKey);
	}
	if (planModeSwitch !== undefined) {
		setCachedChatPlanMode(
			agentId,
			planModeSwitch === "clear" ? undefined : planModeSwitch,
		);
	}
}
