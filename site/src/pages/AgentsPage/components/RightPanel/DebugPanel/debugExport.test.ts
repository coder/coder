import { describe, expect, it } from "vitest";
import {
	buildChatDebugExport,
	buildDebugExportBlob,
	buildRunDebugExport,
	DEBUG_RUN_LIST_LIMIT,
	debugExportFilename,
} from "./debugExport";
import { MockRun } from "./debugFixtures";

const exportedAt = new Date("2026-05-07T10:45:00.000Z");

describe("buildRunDebugExport", () => {
	it("wraps a full debug run in a run-level export envelope", () => {
		const run = MockRun;
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
		const runs = [MockRun, { ...MockRun, id: "run-2" }];
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
		const runs = [MockRun];
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
		const run = MockRun;
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
