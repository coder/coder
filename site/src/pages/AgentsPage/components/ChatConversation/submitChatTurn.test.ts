import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("#/components/Toaster/toast", () => ({
	toast: {
		info: vi.fn(),
		error: vi.fn(),
	},
}));

import type {
	ChatMessage,
	ChatQueuedMessage,
	ChatStatus,
} from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import {
	MockChatMessage,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import { BuiltInCommandPendingError } from "../../hooks/useConversationEditingState";
import { NIL_UUID } from "../../utils/modelOptions";
import {
	createChatStore,
	selectMessagesByID,
	selectOrderedMessageIDs,
	selectQueuedMessages,
} from "./chatStore";
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
): SubmitChatTurnParams => ({
	message: "hello",
	isSubmissionPending: false,
	hasModelOptions: true,
	isEditReasoningEffortDirtyRef: { current: false },
	personalSkills: [],
	workspaceSkills: [],
	compact: vi.fn().mockResolvedValue(undefined),
	clearChatContext: vi.fn().mockResolvedValue(undefined),
	store: createChatStore(),
	agentId: "chat-1",
	clearChatErrorReason: vi.fn(),
	effectiveSelectedModel: pickerModel.id,
	modelOptions: [pickerModel],
	effectiveReasoningEffort: undefined,
	mcpServerIds: ["mcp-1"],
	editMessage: vi.fn().mockResolvedValue({ message: MockChatMessage }),
	sendMessage: vi.fn().mockResolvedValue({ queued: false }),
	onRequestError: vi.fn(),
	invalidateChat: vi.fn(),
	scrollToEnd: vi.fn(),
	setCachedChatPlanMode: vi.fn(),
	...overrides,
});

const textMessage = (
	id: number,
	role: ChatMessage["role"],
	text: string,
): ChatMessage => ({
	...MockChatMessage,
	id,
	role,
	content: [{ type: "text", text }],
});

const seededStore = (
	messages: readonly ChatMessage[],
	status: ChatStatus = "waiting",
) => {
	const store = createChatStore();
	store.seed({
		messages,
		queued_messages: [],
		has_more: false,
		snapshot_version: 1,
	});
	store.applyServerChatStatus(status);
	return store;
};

const visibleTexts = (store: ReturnType<typeof createChatStore>) => {
	const state = store.getSnapshot();
	const byID = selectMessagesByID(state);
	return selectOrderedMessageIDs(state).map((id) => {
		const part = byID.get(id)?.content?.[0];
		return part?.type === "text" ? part.text : undefined;
	});
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

	it.each([
		["compact", "personalSkills"],
		["clear", "organizationSkills"],
	] as const)(
		"throws BuiltInCommandPendingError for /%s while %s is unresolved",
		async (command, unknownList) => {
			const compact = vi.fn();
			const clearChatContext = vi.fn();
			const sendMessage = vi.fn();
			await expect(
				submitChatTurn(
					buildParams({
						message: `/${command}`,
						[unknownList]: undefined,
						compact,
						clearChatContext,
						sendMessage,
					}),
				),
			).rejects.toBeInstanceOf(BuiltInCommandPendingError);
			expect(toast.info).toHaveBeenCalledWith(
				`Checking whether /${command} is available. Try again in a moment.`,
			);
			expect(compact).not.toHaveBeenCalled();
			expect(clearChatContext).not.toHaveBeenCalled();
			expect(sendMessage).not.toHaveBeenCalled();
		},
	);

	it("clears instead of sending when every skill list is known", async () => {
		const clearChatContext = vi.fn().mockResolvedValue(undefined);
		const sendMessage = vi.fn();
		await submitChatTurn(
			buildParams({ message: "/clear", clearChatContext, sendMessage }),
		);
		expect(clearChatContext).toHaveBeenCalledTimes(1);
		expect(sendMessage).not.toHaveBeenCalled();
	});

	it("sends /compact as a message when an organization skill owns the name", async () => {
		const compact = vi.fn();
		const sendMessage = vi.fn().mockResolvedValue({ queued: false });
		await submitChatTurn(
			buildParams({
				message: "/compact",
				organizationSkills: [{ name: "compact" }],
				compact,
				sendMessage,
			}),
		);
		expect(compact).not.toHaveBeenCalled();
		expect(sendMessage).toHaveBeenCalledWith(
			expect.objectContaining({
				content: [{ type: "text", text: "/compact" }],
			}),
		);
	});

	it("reports a failed compaction without sending", async () => {
		const store = seededStore([]);
		const compact = vi.fn().mockRejectedValue(new Error("compact failed"));
		const sendMessage = vi.fn();

		await expect(
			submitChatTurn(
				buildParams({ message: "/compact", store, compact, sendMessage }),
			),
		).rejects.toThrow("compact failed");

		expect(sendMessage).not.toHaveBeenCalled();
		expect(toast.error).toHaveBeenCalled();
	});

	it("edits with a recovered model and scrolls to the end", async () => {
		const store = seededStore([
			{
				...textMessage(5, "user", "old"),
				model_config_id: "stale-model",
			},
		]);
		const editMessage = vi.fn().mockResolvedValue({
			message: textMessage(5, "user", "new text"),
		});
		const scrollToEnd = vi.fn();
		const sendMessage = vi.fn();

		await submitChatTurn(
			buildParams({
				message: "new text",
				editedMessageID: 5,
				store,
				editMessage,
				scrollToEnd,
				sendMessage,
			}),
		);

		expect(editMessage).toHaveBeenCalledWith({
			messageId: 5,
			req: expect.objectContaining({
				content: [{ type: "text", text: "new text" }],
				model_config_id: pickerModel.id,
				mcp_server_ids: ["mcp-1"],
			}),
		});
		expect(scrollToEnd).toHaveBeenCalledWith({ behavior: "smooth" });
		expect(sendMessage).not.toHaveBeenCalled();
	});

	it("omits reasoning effort on edit until the picker is dirty", async () => {
		const store = seededStore([
			{ ...textMessage(5, "user", "old"), model_config_id: pickerModel.id },
		]);
		const editMessage = vi.fn().mockResolvedValue({
			message: textMessage(5, "user", "new text"),
		});
		await submitChatTurn(
			buildParams({
				message: "new text",
				editedMessageID: 5,
				store,
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

	it("shows an edit from submit until the stream reaches it", async () => {
		const store = seededStore([
			textMessage(1, "user", "first"),
			textMessage(2, "assistant", "answer"),
			textMessage(3, "user", "old"),
			textMessage(4, "assistant", "old answer"),
		]);
		store.setQueuedMessages([{ ...MockChatQueuedMessage, id: 8 }]);
		let resolveEdit: () => void = () => {};
		const editMessage = vi.fn(
			() =>
				new Promise<void>((resolve) => {
					resolveEdit = resolve;
				}),
		);

		const submitted = submitChatTurn(
			buildParams({
				message: "new",
				editedMessageID: 3,
				store,
				editMessage,
			}),
		);
		expect(visibleTexts(store)).toEqual(["first", "answer", "new"]);
		expect(selectQueuedMessages(store.getSnapshot())).toEqual([]);

		resolveEdit();
		await submitted;
		// A status does not end the pending edit.
		store.applyServerChatStatus("running");
		expect(visibleTexts(store)).toEqual(["first", "answer", "new"]);
		expect(selectQueuedMessages(store.getSnapshot())).toEqual([]);

		store.batch(() => {
			store.resetHistoryFrom(3);
			store.upsertDurableMessages([textMessage(9, "user", "new")]);
			store.setQueuedMessages([]);
			store.applyServerChatStatus("running");
		});
		expect(visibleTexts(store)).toEqual(["first", "answer", "new"]);
		expect(store.getSnapshot().pendingActions).toEqual([]);
	});

	it("restores the transcript when an edit fails", async () => {
		const store = seededStore([
			textMessage(1, "user", "first"),
			textMessage(2, "assistant", "answer"),
		]);
		const error = new Error("edit failed");
		const onRequestError = vi.fn();

		await expect(
			submitChatTurn(
				buildParams({
					message: "new",
					editedMessageID: 1,
					store,
					editMessage: vi.fn().mockRejectedValue(error),
					onRequestError,
				}),
			),
		).rejects.toThrow("edit failed");

		expect(onRequestError).toHaveBeenCalledWith(error);
		expect(visibleTexts(store)).toEqual(["first", "answer"]);
	});

	it("removes a failed send and reports it", async () => {
		const store = seededStore([textMessage(1, "user", "first")]);
		const error = new Error("send failed");
		const sendMessage = vi.fn().mockRejectedValue(error);
		const onRequestError = vi.fn();
		const invalidateChat = vi.fn();

		await expect(
			submitChatTurn(
				buildParams({ store, sendMessage, onRequestError, invalidateChat }),
			),
		).rejects.toThrow("send failed");

		expect(visibleTexts(store)).toEqual(["first"]);
		expect(onRequestError).toHaveBeenCalledWith(error);
		expect(invalidateChat).toHaveBeenCalledWith("chat-1");
	});

	it("shows a sent message from submit until the stream reports it", async () => {
		const store = seededStore([textMessage(1, "user", "first")]);
		let resolveSend: () => void = () => {};
		const sendMessage = vi.fn(
			() =>
				new Promise<{ queued: boolean }>((resolve) => {
					resolveSend = () => resolve({ queued: false });
				}),
		);

		const submitted = submitChatTurn(buildParams({ store, sendMessage }));
		expect(sendMessage).toHaveBeenCalledWith(
			expect.objectContaining({
				model_config_id: pickerModel.id,
				mcp_server_ids: ["mcp-1"],
			}),
		);
		expect(visibleTexts(store)).toEqual(["first", "hello"]);

		resolveSend();
		await submitted;
		expect(visibleTexts(store)).toEqual(["first", "hello"]);

		store.batch(() => {
			store.upsertDurableMessages([textMessage(9, "user", "hello")]);
			store.applyServerChatStatus("running");
		});
		expect(visibleTexts(store)).toEqual(["first", "hello"]);
		expect(store.getSnapshot().pendingActions).toEqual([]);
	});

	it("shows a send once when the stream reports it before the response", async () => {
		const store = seededStore([textMessage(1, "user", "first")]);
		const sendMessage = vi.fn(async () => {
			store.batch(() => {
				store.upsertDurableMessages([textMessage(9, "user", "hello")]);
				store.applyServerChatStatus("running");
			});
			return { queued: false };
		});

		await submitChatTurn(buildParams({ store, sendMessage }));

		expect(visibleTexts(store)).toEqual(["first", "hello"]);
		expect(store.getSnapshot().pendingActions).toEqual([]);
	});

	it("shows a send to a running chat in the queue until the stream reports it", async () => {
		const store = seededStore([], "running");
		const queued: ChatQueuedMessage = {
			...MockChatQueuedMessage,
			id: 4,
			content: [{ type: "text", text: "hello" }],
		};

		await submitChatTurn(buildParams({ store }));
		expect(
			selectQueuedMessages(store.getSnapshot()).map((row) => row.content),
		).toEqual([queued.content]);

		store.batch(() => {
			store.setQueuedMessages([queued]);
			store.applyServerChatStatus("running");
		});
		expect(selectQueuedMessages(store.getSnapshot())).toEqual([queued]);
		expect(store.getSnapshot().pendingActions).toEqual([]);
	});

	it("clears plan mode when implementing the plan", async () => {
		const setCachedChatPlanMode = vi.fn();
		const sendMessage = vi.fn().mockResolvedValue({ queued: false });

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
		const setCachedChatPlanMode = vi.fn();
		const sendMessage = vi.fn().mockResolvedValue({ queued: false });

		await submitChatTurn(buildParams({ sendMessage, setCachedChatPlanMode }));

		expect(sendMessage).toHaveBeenCalledWith(
			expect.not.objectContaining({ plan_mode: expect.anything() }),
		);
		expect(setCachedChatPlanMode).not.toHaveBeenCalled();
	});
});
