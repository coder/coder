import type { CreateChatMessageRequestWithClearablePlanMode } from "#/api/api";
import { getErrorMessage } from "#/api/errors";
import { buildOptimisticEditedMessage } from "#/api/queries/chatMessageEdits";
import { planModeFieldsForCreateMessage } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
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
import type {
	PendingAttachment,
	PendingWorkspaceUpload,
} from "../ChatPageContent";
import { type ChatStore, selectMessagesByID } from "./chatStore";

export type SubmitChatTurnParams = {
	message: string;
	attachments?: readonly PendingAttachment[];
	workspaceUploads?: readonly PendingWorkspaceUpload[];
	editedMessageID?: number;
	composerParts?: readonly ChatComposerContentPart[];
	clearPlanMode?: boolean;
	isSubmissionPending: boolean;
	hasModelOptions: boolean;
	isEditReasoningEffortDirtyRef: { current: boolean };
	personalSkills: readonly { name: string }[] | undefined;
	organizationSkills: readonly { name: string }[] | undefined;
	projectSkills: readonly { name: string }[] | undefined;
	workspaceSkills: readonly { name: string }[] | undefined;
	compact: () => Promise<unknown>;
	clearChatContext: () => Promise<unknown>;
	store: ChatStore;
	agentId: string;
	clearChatErrorReason: (chatId: string) => void;
	effectiveSelectedModel: string;
	modelOptions: readonly ModelSelectorOption[];
	effectiveReasoningEffort: string | undefined;
	mcpServerIds: readonly string[];
	editMessage: (args: {
		messageId: number;
		req: TypesGen.EditChatMessageRequest;
	}) => Promise<unknown>;
	sendMessage: (
		req: CreateChatMessageRequestWithClearablePlanMode,
	) => Promise<TypesGen.CreateChatMessageResponse>;
	onRequestError: (error: unknown) => void;
	invalidateChat: (chatId: string) => void;
	scrollToEnd: (options: { behavior: "smooth" }) => void;
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

const findBuiltInChatCommand = (
	content: readonly TypesGen.ChatInputPart[],
	editedMessageID: number | undefined,
): (typeof CHAT_SLASH_COMMANDS)[number] | undefined => {
	// Built-ins only intercept new, text-only sends. A skill with the same
	// name takes precedence at availability resolution.
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
	compact,
	clearChatContext,
}: {
	command: (typeof CHAT_SLASH_COMMANDS)[number];
	store: ChatStore;
	compact: () => Promise<unknown>;
	clearChatContext: () => Promise<unknown>;
}): Promise<void> => {
	switch (command.name) {
		case COMPACT_SLASH_COMMAND.name: {
			const pendingID = store.addPendingAction({ kind: "compact" });
			try {
				await compact();
			} catch (error) {
				store.removePendingAction(pendingID);
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

export async function submitChatTurn(
	params: SubmitChatTurnParams,
): Promise<void> {
	const {
		message,
		attachments,
		workspaceUploads,
		editedMessageID,
		composerParts,
		clearPlanMode = false,
		isSubmissionPending,
		hasModelOptions,
		personalSkills,
		organizationSkills,
		projectSkills,
		workspaceSkills,
		compact,
		clearChatContext,
		store,
		agentId,
		clearChatErrorReason,
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
		setCachedChatPlanMode,
	} = params;

	const { content, hasContent } = buildChatInputContent({
		message,
		attachments,
		workspaceUploads,
		composerParts,
	});
	if (!hasContent || isSubmissionPending || !hasModelOptions) {
		return;
	}

	const builtInCommand = findBuiltInChatCommand(content, editedMessageID);
	const builtInCommandResolution = builtInCommand
		? resolveChatSlashCommandAvailability(builtInCommand, [
				{ source: "personal", skills: personalSkills },
				{ source: "org", skills: organizationSkills },
				{ source: "project", skills: projectSkills },
				{ source: "workspace", skills: workspaceSkills },
			])
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
			compact,
			clearChatContext,
		});
		return;
	}

	if (editedMessageID !== undefined) {
		const originalEditedMessage = selectMessagesByID(store.getSnapshot()).get(
			editedMessageID,
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
		const pendingID = store.addPendingAction({
			kind: "edit",
			editedMessageID,
			message:
				originalEditedMessage &&
				buildOptimisticEditedMessage({
					requestContent: request.content,
					originalMessage: originalEditedMessage,
					attachmentMediaTypes: buildAttachmentMediaTypes(attachments),
				}),
		});
		try {
			await editMessage({
				messageId: editedMessageID,
				req: request,
			});
		} catch (error) {
			store.removePendingAction(pendingID);
			onRequestError(error);
			invalidateChat(agentId);
			throw error;
		}
		scrollToEnd({ behavior: "smooth" });
		return;
	}

	const selectedModelConfigID = effectiveSelectedModel || undefined;
	const request: CreateChatMessageRequestWithClearablePlanMode = {
		content,
		model_config_id: selectedModelConfigID,
		reasoning_effort: effectiveReasoningEffort,
		mcp_server_ids: [...mcpServerIds],
		...planModeFieldsForCreateMessage(clearPlanMode),
	};
	clearChatErrorReason(agentId);
	store.clearStreamError();

	const pendingID = store.addPendingAction({
		kind: "send",
		message: buildOptimisticEditedMessage({
			requestContent: content,
			originalMessage: {
				id: 0,
				chat_id: agentId,
				created_at: new Date().toISOString(),
				role: "user",
				model_config_id: selectedModelConfigID,
			},
			attachmentMediaTypes: buildAttachmentMediaTypes(attachments),
		}),
	});
	try {
		await sendMessage(request);
	} catch (error) {
		store.removePendingAction(pendingID);
		onRequestError(error);
		invalidateChat(agentId);
		throw error;
	}
	if (clearPlanMode) {
		setCachedChatPlanMode(agentId, undefined);
	}
}
