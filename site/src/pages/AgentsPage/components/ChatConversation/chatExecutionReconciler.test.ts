import type { ChatExecutionSnapshotEvent } from "#/api/queries/chats";
import {
	type ChatExecutionReconcilerPorts,
	reconcileChatExecutionSnapshotEvent,
} from "./chatExecutionReconciler";

const createPorts = (): ChatExecutionReconcilerPorts => ({
	applyPreviewPart: vi.fn(),
	beginHistoryReplacement: vi.fn(),
	resetPreview: vi.fn(),
	commitMessage: vi.fn(),
	replaceQueue: vi.fn(),
	applyActionRequired: vi.fn(),
	applyStatus: vi.fn(),
	applyError: vi.fn(),
	applyRetry: vi.fn(),
});

const reconcile = (
	event: ChatExecutionSnapshotEvent,
	ports = createPorts(),
) => {
	reconcileChatExecutionSnapshotEvent({ event, connectionEpoch: 7, ports });
	return ports;
};

describe(reconcileChatExecutionSnapshotEvent.name, () => {
	it("routes preview parts with their connection and backend episode identity", () => {
		const ports = reconcile({
			type: "message_part",
			chat_id: "chat-1",
			message_part: {
				role: "assistant",
				history_version: 3,
				generation_attempt: 2,
				seq: 4,
				part: { type: "text", text: "hello" },
			},
		});

		expect(ports.applyPreviewPart).toHaveBeenCalledWith({
			connectionEpoch: 7,
			historyVersion: 3,
			generationAttempt: 2,
			seq: 4,
			role: "assistant",
			part: { type: "text", text: "hello" },
		});
	});

	it("routes durable snapshot projections to their role-specific ports", () => {
		const ports = createPorts();
		const toolCalls = [
			{ tool_call_id: "call-1", tool_name: "custom_tool", args: "{}" },
		];
		const events: ChatExecutionSnapshotEvent[] = [
			{ type: "history_reset", chat_id: "chat-1" },
			{ type: "preview_reset", chat_id: "chat-1" },
			{
				type: "message",
				chat_id: "chat-1",
				message: {
					id: 1,
					chat_id: "chat-1",
					role: "assistant",
					content: [],
					created_at: "2026-01-01T00:00:00Z",
				},
			},
			{ type: "queue_update", chat_id: "chat-1", queued_messages: [] },
			{
				type: "action_required",
				chat_id: "chat-1",
				action_required: { tool_calls: toolCalls },
			},
			{ type: "status", chat_id: "chat-1", status: { status: "waiting" } },
			{ type: "error", chat_id: "chat-1" },
			{
				type: "retry",
				chat_id: "chat-1",
				retry: {
					attempt: 1,
					delay_ms: 1000,
					error: "retry",
					retrying_at: "2026-01-01T00:00:00Z",
				},
			},
		];

		for (const event of events) {
			reconcile(event, ports);
		}

		expect(ports.beginHistoryReplacement).toHaveBeenCalledOnce();
		expect(ports.resetPreview).toHaveBeenCalledOnce();
		expect(ports.commitMessage).toHaveBeenCalledWith(events[2].message);
		expect(ports.replaceQueue).toHaveBeenCalledWith([]);
		expect(ports.applyActionRequired).toHaveBeenCalledWith({
			tool_calls: toolCalls,
		});
		expect(ports.applyStatus).toHaveBeenCalledWith("waiting");
		expect(ports.applyError).toHaveBeenCalledWith(undefined);
		expect(ports.applyRetry).toHaveBeenCalledWith(events[7].retry);
	});

	it("treats a queue snapshot without messages as an empty authoritative queue", () => {
		const ports = reconcile({ type: "queue_update", chat_id: "chat-1" });
		expect(ports.replaceQueue).toHaveBeenCalledWith([]);
	});
});
