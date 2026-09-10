import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("sonner", () => ({
	toast: {
		info: vi.fn(),
		error: vi.fn(),
	},
}));

import { toast } from "sonner";
import type { ChatMessage } from "#/api/typesGenerated";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import {
	MockChatMessage,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import { BuiltInCommandPendingError } from "../../hooks/useConversationEditingState";
import { NIL_UUID } from "../../utils/modelOptions";
import { createChatStore } from "./chatStore";
import {
	lastModelConfigIDStorageKey,
	resolveEditModelConfigID,
	type SubmitChatTurnParams,
	submitChatTurn,
} from "./submitChatTurn";

const buildOption = (id: string): ModelSelectorOption => ({
	id,
	provider: "openai",
	model: id,
	displayName: id,
});

const pickerModel = buildOption("picker-model");
const originalModel = buildOption("original-model");

const buildParams = (
	overrides: Partial<SubmitChatTurnParams> = {},
): SubmitChatTurnParams => {
	const store = overrides.store ?? createChatStore();
	if (!overrides.store) {
		store.setActiveChatID("chat-1");
	}
	return {
		message: "hello",
		isSubmissionPending: false,
		hasModelOptions: true,
		pendingPlanModeSyncRef: { current: null },
		pendingWorkspaceSyncRef: { current: null },
		isEditReasoningEffortDirtyRef: { current: false },
		personalSkills: [],
		workspaceSkills: [],
		compact: vi.fn().mockResolvedValue(undefined),
		clearChatContext: vi.fn().mockResolvedValue(undefined),
		store,
		agentId: "chat-1",
		clearChatErrorReason: vi.fn(),
		acceptServerChatStatus: vi.fn(),
		chatMessages: undefined,
		effectiveSelectedModel: pickerModel.id,
		modelOptions: [pickerModel],
		effectiveReasoningEffort: undefined,
		mcpServerIds: ["mcp-1"],
		editMessage: vi.fn().mockResolvedValue(undefined),
		sendMessage: vi.fn().mockResolvedValue({ queued: false }),
		onRequestError: vi.fn(),
		invalidateChat: vi.fn(),
		scrollToEnd: vi.fn(),
		upsertCacheMessages: vi.fn(),
		getCacheQueuedMessages: vi.fn(),
		setCacheQueuedMessages: vi.fn(),
		fetchQueueConvergence: vi.fn().mockResolvedValue({
			messages: [],
			queued_messages: [],
			has_more: false,
		}),
		setCachedChatPlanMode: vi.fn(),
		...overrides,
	};
};

describe("resolveEditModelConfigID", () => {
	it("uses the picker when the original model is no longer available", () => {
		expect(
			resolveEditModelConfigID({
				pickerModelConfigID: pickerModel.id,
				originalModelConfigID: "stale-model",
				modelOptions: [pickerModel],
			}),
		).toBe(pickerModel.id);
	});

	it("uses the picker when the user changed a still-selectable model", () => {
		expect(
			resolveEditModelConfigID({
				pickerModelConfigID: pickerModel.id,
				originalModelConfigID: originalModel.id,
				modelOptions: [pickerModel, originalModel],
			}),
		).toBe(pickerModel.id);
	});

	it("omits the model so the backend keeps a selectable original", () => {
		expect(
			resolveEditModelConfigID({
				pickerModelConfigID: originalModel.id,
				originalModelConfigID: originalModel.id,
				modelOptions: [originalModel],
			}),
		).toBeUndefined();
	});

	it("omits blank and unset picker or original references", () => {
		expect(
			resolveEditModelConfigID({
				pickerModelConfigID: undefined,
				originalModelConfigID: originalModel.id,
				modelOptions: [originalModel],
			}),
		).toBeUndefined();
		expect(
			resolveEditModelConfigID({
				pickerModelConfigID: pickerModel.id,
				originalModelConfigID: undefined,
				modelOptions: [pickerModel],
			}),
		).toBeUndefined();
		expect(
			resolveEditModelConfigID({
				pickerModelConfigID: pickerModel.id,
				originalModelConfigID: NIL_UUID,
				modelOptions: [pickerModel],
			}),
		).toBeUndefined();
	});
});

describe("submitChatTurn", () => {
	beforeEach(() => {
		localStorage.clear();
		vi.clearAllMocks();
	});

	it("returns without sending when content, models, or a pending submit block it", async () => {
		const sendMessage = vi.fn();
		await submitChatTurn(buildParams({ message: "   ", sendMessage }));
		await submitChatTurn(buildParams({ hasModelOptions: false, sendMessage }));
		await submitChatTurn(
			buildParams({ isSubmissionPending: true, sendMessage }),
		);
		expect(sendMessage).not.toHaveBeenCalled();
	});

	it("waits for pending chat-setting syncs before sending", async () => {
		const planModeUpdate = createDeferred<void>();
		const sendMessage = vi.fn().mockResolvedValue({ queued: false });
		const turn = submitChatTurn(
			buildParams({
				pendingPlanModeSyncRef: { current: planModeUpdate.promise },
				sendMessage,
			}),
		);
		await Promise.resolve();
		expect(sendMessage).not.toHaveBeenCalled();
		planModeUpdate.resolve(undefined);
		await turn;
		expect(sendMessage).toHaveBeenCalledTimes(1);
	});

	it("throws BuiltInCommandPendingError while slash-command skills are unresolved", async () => {
		const compact = vi.fn();
		const sendMessage = vi.fn();
		await expect(
			submitChatTurn(
				buildParams({
					message: "/compact",
					personalSkills: undefined,
					workspaceSkills: [],
					compact,
					sendMessage,
				}),
			),
		).rejects.toBeInstanceOf(BuiltInCommandPendingError);
		expect(toast.info).toHaveBeenCalledWith(
			"Checking whether /compact is available. Try again in a moment.",
		);
		expect(compact).not.toHaveBeenCalled();
		expect(sendMessage).not.toHaveBeenCalled();
	});

	it("compacts instead of sending and restores state if compact fails", async () => {
		const store = createChatStore();
		store.setActiveChatID("chat-1");
		store.setChatStatus("waiting");
		const compact = vi.fn().mockRejectedValue(new Error("compact failed"));
		const sendMessage = vi.fn();

		await expect(
			submitChatTurn(
				buildParams({
					message: "/compact",
					store,
					compact,
					sendMessage,
				}),
			),
		).rejects.toThrow("compact failed");

		expect(compact).toHaveBeenCalledTimes(1);
		expect(sendMessage).not.toHaveBeenCalled();
		expect(store.getSnapshot().chatStatus).toBe("waiting");
		expect(toast.error).toHaveBeenCalled();
	});

	it("edits with a recovered model and scrolls to the end", async () => {
		const originalMessage: ChatMessage = {
			...MockChatMessage,
			id: 5,
			model_config_id: "stale-model",
			content: [{ type: "text", text: "old" }],
		};
		const editMessage = vi.fn().mockResolvedValue(undefined);
		const scrollToEnd = vi.fn();
		const sendMessage = vi.fn();

		await submitChatTurn(
			buildParams({
				message: "new text",
				editedMessageID: 5,
				chatMessages: [originalMessage],
				editMessage,
				scrollToEnd,
				sendMessage,
			}),
		);

		expect(editMessage).toHaveBeenCalledWith({
			messageId: 5,
			optimisticMessage: expect.objectContaining({
				id: 5,
				content: [{ type: "text", text: "new text" }],
			}),
			req: expect.objectContaining({
				model_config_id: pickerModel.id,
				mcp_server_ids: ["mcp-1"],
			}),
		});
		expect(scrollToEnd).toHaveBeenCalledWith({ behavior: "smooth" });
		expect(sendMessage).not.toHaveBeenCalled();
		expect(localStorage.getItem(lastModelConfigIDStorageKey)).toBe(
			pickerModel.id,
		);
	});

	it("omits reasoning effort on edit until the picker is dirty", async () => {
		const originalMessage = {
			...MockChatMessage,
			id: 5,
			model_config_id: pickerModel.id,
		};
		const editMessage = vi.fn().mockResolvedValue(undefined);
		await submitChatTurn(
			buildParams({
				message: "new text",
				editedMessageID: 5,
				chatMessages: [originalMessage],
				effectiveReasoningEffort: "high",
				isEditReasoningEffortDirtyRef: { current: false },
				editMessage,
			}),
		);
		expect(editMessage).toHaveBeenCalledWith(
			expect.objectContaining({
				req: expect.objectContaining({
					reasoning_effort: undefined,
					model_config_id: undefined,
				}),
			}),
		);
	});

	it("reports send failures and invalidates the chat", async () => {
		const error = new Error("send failed");
		const sendMessage = vi.fn().mockRejectedValue(error);
		const onRequestError = vi.fn();
		const acceptServerChatStatus = vi.fn();
		const invalidateChat = vi.fn();

		await expect(
			submitChatTurn(
				buildParams({
					sendMessage,
					onRequestError,
					acceptServerChatStatus,
					invalidateChat,
				}),
			),
		).rejects.toThrow("send failed");

		expect(onRequestError).toHaveBeenCalledWith(error);
		expect(acceptServerChatStatus).toHaveBeenCalledTimes(1);
		expect(invalidateChat).toHaveBeenCalledWith("chat-1");
	});

	it("upserts a non-queued send, persists the model, and sets running", async () => {
		const store = createChatStore();
		store.setActiveChatID("chat-1");
		const inserted = { ...MockChatMessage, id: 9 };
		const sendMessage = vi.fn().mockResolvedValue({
			queued: false,
			message: inserted,
		});
		const upsertCacheMessages = vi.fn();

		await submitChatTurn(
			buildParams({
				store,
				sendMessage,
				upsertCacheMessages,
			}),
		);

		expect(sendMessage).toHaveBeenCalledWith(
			expect.objectContaining({
				model_config_id: pickerModel.id,
				mcp_server_ids: ["mcp-1"],
			}),
		);
		expect(upsertCacheMessages).toHaveBeenCalledWith([inserted]);
		expect(store.getSnapshot().chatStatus).toBe("running");
		expect(localStorage.getItem(lastModelConfigIDStorageKey)).toBe(
			pickerModel.id,
		);
	});

	it("reconciles a queued send and clears plan mode on implement", async () => {
		const store = createChatStore();
		store.setActiveChatID("chat-1");
		const queuedHead = { ...MockChatQueuedMessage, id: 3 };
		store.setQueuedMessages([queuedHead]);
		const promotedUser: ChatMessage = {
			...MockChatMessage,
			id: 10,
			role: "user",
		};
		const sendMessage = vi.fn().mockResolvedValue({
			queued: true,
			messages: [promotedUser],
			queued_message: { ...MockChatQueuedMessage, id: 4 },
		});
		const fetchQueueConvergence = vi.fn().mockResolvedValue({
			messages: [],
			has_more: false,
			queued_messages: [],
		});
		const setCacheQueuedMessages = vi.fn();
		const setCachedChatPlanMode = vi.fn();

		await submitChatTurn(
			buildParams({
				message: "Implement the plan.",
				planModeSwitch: "clear",
				store,
				sendMessage,
				setCacheQueuedMessages,
				setCachedChatPlanMode,
				fetchQueueConvergence,
			}),
		);

		expect(setCacheQueuedMessages).toHaveBeenCalled();
		expect(setCachedChatPlanMode).toHaveBeenCalledWith("chat-1", undefined);
		expect(sendMessage).toHaveBeenCalledWith(
			expect.objectContaining({
				plan_mode: "",
			}),
		);
		await vi.waitFor(() => {
			expect(fetchQueueConvergence).toHaveBeenCalledWith("chat-1");
		});
	});
});
