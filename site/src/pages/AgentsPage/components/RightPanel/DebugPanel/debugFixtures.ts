import type { ChatDebugRun, ChatDebugStep } from "#/api/typesGenerated";

export const CHAT_ID = "debug-chat-1";

export const MockStep: ChatDebugStep = {
	id: "step-1",
	run_id: "run-1",
	chat_id: CHAT_ID,
	step_number: 1,
	operation: "stream",
	status: "completed",
	normalized_request: { model: "gpt-4", prompt: "Hello" },
	normalized_response: { content: "Hi there!", finish_reason: "stop" },
	usage: { prompt_tokens: "10", completion_tokens: "5", total_tokens: "15" },
	attempts: [],
	metadata: { provider: "openai" },
	started_at: "2026-03-05T12:00:06Z",
	updated_at: "2026-03-05T12:00:08Z",
	finished_at: "2026-03-05T12:00:08Z",
};

export const MockRun: ChatDebugRun = {
	id: "run-1",
	chat_id: CHAT_ID,
	kind: "chat_turn",
	status: "completed",
	provider: "openai",
	model: "gpt-4",
	summary: { result: "Generated response successfully" },
	started_at: "2026-03-05T12:00:05Z",
	updated_at: "2026-03-05T12:00:08Z",
	finished_at: "2026-03-05T12:00:08Z",
	steps: [MockStep],
};
