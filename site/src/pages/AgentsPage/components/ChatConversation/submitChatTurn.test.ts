import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("#/components/Toaster/toast", () => ({
	toast: {
		info: vi.fn(),
		error: vi.fn(),
	},
}));

import type { ChatMessage } from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import type { ModelSelectorOption } from "#/modules/aiModels/ModelSelector";
import {
	MockChat,
	MockChatMessage,
	MockChatQueuedMessage,
} from "#/testHelpers/chatEntities";
import { BuiltInCommandPendingError } from "../../hooks/useConversationEditingState";
import { NIL_UUID } from "../../utils/modelOptions";
import {
	createChatStore,
	selectChatStatus,
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
	compact: vi.fn().mockResolvedValue(MockChat),
	clearChatContext: vi.fn().mockResolvedValue(undefined),
	store: createChatStore(),
	agentId: "chat-1",
	clearChatErrorReason: vi.fn(),
	effectiveSelectedModel: pickerModel.id,
	modelOptions: [pickerModel],
	effectiveReasoningEffort: undefined,
	mcpServerIds: ["mcp-1"],
	editMessage: vi
		.fn()
		.mockResolvedValue({ message: MockChatMessage, snapshot_version: 2 }),
	sendMessage: vi
		.fn()
		.mockResolvedValue({ queued: false, snapshot_version: 2 }),
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

const seededStore = (messages: readonly ChatMessage[]) => {
	const store = createChatStore();
	store.seed({
		messages,
		queued_messages: [],
		has_more: false,
		snapshot_version: 1,
	});
	store.setChatStatus("waiting");
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

	it("shows a compaction as running until the stream reaches it", async () => {
		const store = seededStore([]);
		let resolveCompact: (chat: typeof MockChat) => void = () => {};
		const compact = vi.fn(
			() =>
				new Promise<typeof MockChat>((resolve) => {
					resolveCompact = resolve;
				}),
		);

		const submitted = submitChatTurn(
			buildParams({ message: "/compact", store, compact }),
		);
		expect(selectChatStatus(store.getSnapshot())).toBe("running");

		resolveCompact({ ...MockChat, snapshot_version: 4 });
		await submitted;
		store.applyCheckpoint(3);
		expect(selectChatStatus(store.getSnapshot())).toBe("running");
		store.applyCheckpoint(4);
		expect(selectChatStatus(store.getSnapshot())).toBe("waiting");
	});

	it("restores the status when a compaction fails", async () => {
		const store = seededStore([]);
		const compact = vi.fn().mockRejectedValue(new Error("compact failed"));
		const sendMessage = vi.fn();

		await expect(
			submitChatTurn(
				buildParams({ message: "/compact", store, compact, sendMessage }),
			),
		).rejects.toThrow("compact failed");

		expect(sendMessage).not.toHaveBeenCalled();
		expect(selectChatStatus(store.getSnapshot())).toBe("waiting");
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
			snapshot_version: 2,
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
			snapshot_version: 2,
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
		let resolveEdit: (value: {
			message: ChatMessage;
			snapshot_version: number;
		}) => void = () => {};
		const editMessage = vi.fn(
			() =>
				new Promise<{ message: ChatMessage; snapshot_version: number }>(
					(resolve) => {
						resolveEdit = resolve;
					},
				),
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
		expect(selectChatStatus(store.getSnapshot())).toBe("running");

		resolveEdit({
			message: textMessage(9, "user", "new"),
			snapshot_version: 6,
		});
		await submitted;
		expect(visibleTexts(store)).toEqual(["first", "answer", "new"]);

		// The stream applies the edit, then its checkpoint ends the overlay.
		store.resetHistoryFrom(3);
		store.upsertDurableMessages([textMessage(9, "user", "new")]);
		store.setQueuedMessages([]);
		store.setChatStatus("running");
		store.applyCheckpoint(6);
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
		expect(selectChatStatus(store.getSnapshot())).toBe("waiting");
	});

	it("reports send failures and invalidates the chat", async () => {
		const error = new Error("send failed");
		const sendMessage = vi.fn().mockRejectedValue(error);
		const onRequestError = vi.fn();
		const invalidateChat = vi.fn();

		await expect(
			submitChatTurn(
				buildParams({ sendMessage, onRequestError, invalidateChat }),
			),
		).rejects.toThrow("send failed");

		expect(onRequestError).toHaveBeenCalledWith(error);
		expect(invalidateChat).toHaveBeenCalledWith("chat-1");
	});

	it("shows a sent message and a running turn until the stream reaches it", async () => {
		const store = seededStore([textMessage(1, "user", "first")]);
		const sendMessage = vi.fn().mockResolvedValue({
			queued: false,
			message: textMessage(9, "user", "hello"),
			snapshot_version: 5,
		});

		await submitChatTurn(buildParams({ store, sendMessage }));

		expect(sendMessage).toHaveBeenCalledWith(
			expect.objectContaining({
				model_config_id: pickerModel.id,
				mcp_server_ids: ["mcp-1"],
			}),
		);
		expect(visibleTexts(store)).toEqual(["first", "hello"]);
		expect(selectChatStatus(store.getSnapshot())).toBe("running");

		store.applyCheckpoint(5);
		expect(visibleTexts(store)).toEqual(["first"]);
		expect(selectChatStatus(store.getSnapshot())).toBe("waiting");
	});

	it("adds nothing for a send the stream already reached", async () => {
		const store = seededStore([textMessage(1, "user", "first")]);
		store.upsertDurableMessage(textMessage(9, "user", "hello"));
		store.applyCheckpoint(5);
		const sendMessage = vi.fn().mockResolvedValue({
			queued: false,
			message: textMessage(9, "user", "hello"),
			snapshot_version: 5,
		});

		await submitChatTurn(buildParams({ store, sendMessage }));

		expect(store.getSnapshot().pendingActions).toEqual([]);
		expect(visibleTexts(store)).toEqual(["first", "hello"]);
	});

	it("shows a queued send in the queue until the stream reaches it", async () => {
		const store = seededStore([]);
		const queued = { ...MockChatQueuedMessage, id: 4 };
		const sendMessage = vi.fn().mockResolvedValue({
			queued: true,
			queued_message: queued,
			snapshot_version: 7,
		});

		await submitChatTurn(buildParams({ store, sendMessage }));
		expect(selectQueuedMessages(store.getSnapshot())).toEqual([queued]);

		store.applyCheckpoint(7);
		expect(selectQueuedMessages(store.getSnapshot())).toEqual([]);
	});

	it("clears plan mode when implementing the plan", async () => {
		const setCachedChatPlanMode = vi.fn();
		const sendMessage = vi
			.fn()
			.mockResolvedValue({ queued: false, snapshot_version: 2 });

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
		const sendMessage = vi
			.fn()
			.mockResolvedValue({ queued: false, snapshot_version: 2 });

		await submitChatTurn(buildParams({ sendMessage, setCachedChatPlanMode }));

		expect(sendMessage).toHaveBeenCalledWith(
			expect.not.objectContaining({ plan_mode: expect.anything() }),
		);
		expect(setCachedChatPlanMode).not.toHaveBeenCalled();
	});
});
