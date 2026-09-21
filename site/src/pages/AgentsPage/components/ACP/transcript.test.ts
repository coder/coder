import { expect, it } from "vitest";
import { MockACPSession } from "#/testHelpers/acp";
import { acpTranscript } from "./transcript";

it("preserves ACP text and tool execution state for the shared renderer", () => {
	const transcript = acpTranscript(MockACPSession);
	expect(transcript[0].parsed.markdown).toBe(MockACPSession.entries[0].text);
	const tool = transcript.flatMap((message) => message.parsed.tools)[0];
	expect(tool.status).toBe("running");
	expect(tool.args).toEqual({ command: "go test ./agent/..." });
	const finished = acpTranscript({
		...MockACPSession,
		entries: MockACPSession.entries.map((entry) =>
			entry.kind === "tool"
				? { ...entry, status: "failed", text: "Test failed" }
				: entry,
		),
	});
	expect(finished.flatMap((message) => message.parsed.tools)[0]).toMatchObject({
		status: "error",
		isError: true,
		result: "Test failed",
	});
});
