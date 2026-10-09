import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("sonner", () => ({
	toast: {
		info: vi.fn(),
		error: vi.fn(),
	},
}));

import { toast } from "sonner";
import type {
	ChatMessage,
	EditChatMessageResponse,
} from "#/api/typesGenerated";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import {
	MockChatMessage,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import { BuiltInCommandPendingError } from "../../hooks/useConversationEditingState";
import { NIL_UUID } from "../../utils/modelOptions";
import { createChatStore, visibleMessages } from "./chatStore";
import {
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
		editMessage: vi.fn().mockResolvedValue({ message: MockChatMessage }),
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
		const editMessage = vi.fn().mockResolvedValue({ message: MockChatMessage });
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
			req: expect.objectContaining({
				model_config_id: pickerModel.id,
				mcp_server_ids: ["mcp-1"],
			}),
		});
		expect(scrollToEnd).toHaveBeenCalledWith({ behavior: "smooth" });
		expect(sendMessage).not.toHaveBeenCalled();
	});

	describe("while an edit is pending", () => {
		const question: ChatMessage = {
			...MockChatMessage,
			id: 5,
			role: "user",
			content: [{ type: "text", text: "old" }],
		};
		const answer: ChatMessage = {
			...MockChatMessage,
			id: 6,
			role: "assistant",
			content: [{ type: "text", text: "old answer" }],
		};
		const shown = (store: ReturnType<typeof createChatStore>) => {
			const { orderedMessageIDs, messagesByID, pendingEdit } =
				store.getSnapshot();
			return visibleMessages(orderedMessageIDs, messagesByID, pendingEdit).map(
				(message) => {
					const part = message.content?.[0];
					return `${message.id}:${part?.type === "text" ? part.text : ""}`;
				},
			);
		};
		const startEdit = () => {
			const store = createChatStore();
			store.setActiveChatID("chat-1");
			store.upsertDurableMessages([question, answer]);
			const response = createDeferred<EditChatMessageResponse>();
			const params = buildParams({
				store,
				message: "new text",
				editedMessageID: 5,
				chatMessages: [question, answer],
				editMessage: vi.fn().mockReturnValue(response.promise),
			});
			const submitted = submitChatTurn(params);
			return { store, params, response, submitted };
		};

		const replacement: ChatMessage = {
			...question,
			id: 7,
			content: [{ type: "text", text: "new text" }],
		};

		it("shows the edited text in place of the edited message and hides the rest until the stream delivers the edit", async () => {
			const { store, response, submitted } = startEdit();

			expect(shown(store)).toEqual(["5:new text"]);

			response.resolve({ message: replacement, deleted_message_ids: [5, 6] });
			await submitted;

			expect(shown(store)).toEqual(["5:new text"]);

			// The stream's history_reset for the edit.
			store.replaceMessages([replacement]);

			expect(shown(store)).toEqual(["7:new text"]);
		});

		it("does not bring back a message that a later edit deleted when the edit response arrives late", async () => {
			const { store, response, submitted } = startEdit();
			// The stream delivers the edit, then another tab's edit of its result.
			store.replaceMessages([replacement]);
			store.replaceMessages([
				{ ...replacement, id: 8, content: [{ type: "text", text: "newer" }] },
			]);

			response.resolve({ message: replacement, deleted_message_ids: [5, 6] });
			await submitted;

			expect(shown(store)).toEqual(["8:newer"]);
		});

		it("shows every stored message again and reports the error when the edit fails", async () => {
			const { store, params, response, submitted } = startEdit();
			// The turn the edit would replace commits a message meanwhile.
			store.upsertDurableMessage({ ...answer, id: 8 });

			const error = new Error("edit rejected");
			response.reject(error);
			await expect(submitted).rejects.toThrow("edit rejected");

			expect(shown(store)).toEqual(["5:old", "6:old answer", "8:old answer"]);
			expect(params.onRequestError).toHaveBeenCalledWith(error);
			expect(params.acceptServerChatStatus).toHaveBeenCalled();
			expect(params.invalidateChat).toHaveBeenCalledWith("chat-1");
		});
	});

	it("omits reasoning effort on edit until the picker is dirty", async () => {
		const originalMessage = {
			...MockChatMessage,
			id: 5,
			model_config_id: pickerModel.id,
		};
		const editMessage = vi.fn().mockResolvedValue({ message: MockChatMessage });
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

	it("upserts a non-queued send and sets running", async () => {
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
	});

	it("reconciles a queued send", async () => {
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

		await submitChatTurn(
			buildParams({
				store,
				sendMessage,
				setCacheQueuedMessages,
				fetchQueueConvergence,
			}),
		);

		expect(setCacheQueuedMessages).toHaveBeenCalled();
		await vi.waitFor(() => {
			expect(fetchQueueConvergence).toHaveBeenCalledWith("chat-1");
		});
	});

	it("clears plan mode when implementing the plan", async () => {
		const sendMessage = vi.fn().mockResolvedValue({ queued: false });
		const setCachedChatPlanMode = vi.fn();

		await submitChatTurn(
			buildParams({
				message: "Implement the plan.",
				clearPlanMode: true,
				sendMessage,
				setCachedChatPlanMode,
			}),
		);

		expect(sendMessage).toHaveBeenCalledWith(
			expect.objectContaining({ plan_mode: "" }),
		);
		expect(setCachedChatPlanMode).toHaveBeenCalledWith("chat-1", undefined);
	});

	it("leaves plan mode untouched on an ordinary send", async () => {
		const sendMessage = vi.fn().mockResolvedValue({ queued: false });
		const setCachedChatPlanMode = vi.fn();

		await submitChatTurn(buildParams({ sendMessage, setCachedChatPlanMode }));

		expect(sendMessage).toHaveBeenCalledWith(
			expect.not.objectContaining({ plan_mode: expect.anything() }),
		);
		expect(setCachedChatPlanMode).not.toHaveBeenCalled();
	});
});
