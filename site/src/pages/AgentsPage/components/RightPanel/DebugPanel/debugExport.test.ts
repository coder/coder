import { describe, expect, it } from "vitest";
import type { ChatDebugRun } from "#/api/typesGenerated";
import {
	buildChatDebugExport,
	buildDebugExportBlob,
	buildRunDebugExport,
	DEBUG_RUN_LIST_LIMIT,
	debugExportFilename,
} from "./debugExport";

const exportedAt = new Date("2026-05-07T10:45:00.000Z");

const makeRun = (overrides: Partial<ChatDebugRun> = {}): ChatDebugRun => ({
	id: "11111111-1111-1111-1111-111111111111",
	chat_id: "00000000-0000-0000-0000-000000000000",
	kind: "chat_turn",
	status: "completed",
	provider: "openai",
	model: "gpt-4",
	summary: { first_message: "Help debug this" },
	started_at: "2026-05-07T10:40:00Z",
	updated_at: "2026-05-07T10:44:00Z",
	finished_at: "2026-05-07T10:44:00Z",
	steps: [
		{
			id: "22222222-2222-2222-2222-222222222222",
			run_id: "11111111-1111-1111-1111-111111111111",
			chat_id: "00000000-0000-0000-0000-000000000000",
			step_number: 1,
			operation: "stream",
			status: "completed",
			normalized_request: { messages: [{ role: "user", content: "hello" }] },
			normalized_response: { content: "hi" },
			usage: { prompt_tokens: 10, completion_tokens: 2 },
			attempts: [
				{
					number: 1,
					status: "completed",
					request_headers: { Authorization: "[REDACTED]" },
				},
			],
			metadata: { provider: "openai" },
			started_at: "2026-05-07T10:41:00Z",
			updated_at: "2026-05-07T10:42:00Z",
			finished_at: "2026-05-07T10:42:00Z",
		},
	],
	...overrides,
});

describe("buildRunDebugExport", () => {
	it("wraps a full debug run in a run-level export envelope", () => {
		const run = makeRun();
		const payload = buildRunDebugExport(run.chat_id, run, exportedAt);

		expect(payload).toEqual({
			version: 1,
			scope: "run",
			exported_at: "2026-05-07T10:45:00.000Z",
			chat_id: run.chat_id,
			run_id: run.id,
			run,
		});
	});
});

describe("buildChatDebugExport", () => {
	it("wraps full debug runs in a chat-level export envelope", () => {
		const runs = [makeRun(), makeRun({ id: "run-2" })];
		const payload = buildChatDebugExport(runs[0].chat_id, runs, exportedAt);

		expect(payload.version).toBe(1);
		expect(payload.scope).toBe("chat");
		expect(payload.exported_at).toBe("2026-05-07T10:45:00.000Z");
		expect(payload.chat_id).toBe(runs[0].chat_id);
		expect(payload.run_count).toBe(2);
		expect(payload.requested_run_count).toBe(2);
		expect(payload.limited_to_most_recent).toBe(DEBUG_RUN_LIST_LIMIT);
		expect(payload.runs).toEqual(runs);
		expect(payload.runs[0].steps).toHaveLength(1);
		expect(payload).not.toHaveProperty("failed_runs");
	});

	it("includes failed run metadata when some detail fetches fail", () => {
		const runs = [makeRun()];
		const failedRuns = [{ run_id: "run-2", message: "not found" }];
		const payload = buildChatDebugExport(runs[0].chat_id, runs, exportedAt, {
			failedRuns,
			requestedRunCount: 2,
		});

		expect(payload.run_count).toBe(1);
		expect(payload.requested_run_count).toBe(2);
		expect(payload.failed_runs).toEqual(failedRuns);
	});
});

describe("buildDebugExportBlob", () => {
	it("serializes export payloads as formatted JSON blobs", async () => {
		const run = makeRun();
		const payload = buildRunDebugExport(run.chat_id, run, exportedAt);
		const blob = buildDebugExportBlob(payload);

		expect(blob.type).toBe("application/json");
		const parsed = JSON.parse(await blob.text());
		expect(parsed).toMatchObject({
			version: 1,
			scope: "run",
			chat_id: run.chat_id,
			run_id: run.id,
		});
		expect(parsed.run.steps).toHaveLength(1);
	});
});

describe("debugExportFilename", () => {
	it("generates a chat-level filename", () => {
		expect(
			debugExportFilename({
				chatId: "abcdef12-3456-7890-abcd-ef1234567890",
				exportedAt,
			}),
		).toBe("coder-agents-debug-chat-abcdef12-2026-05-07T10-45-00-000Z.json");
	});

	it("generates a run-level filename", () => {
		expect(
			debugExportFilename({
				chatId: "abcdef12-3456-7890-abcd-ef1234567890",
				runId: "deadbeef-1234-5678-9abc-def012345678",
				exportedAt,
			}),
		).toBe("coder-agents-debug-run-deadbeef-2026-05-07T10-45-00-000Z.json");
	});
});
