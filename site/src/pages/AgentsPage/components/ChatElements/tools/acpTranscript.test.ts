import { describe, expect, it } from "vitest";
import { MockACPTranscript } from "#/testHelpers/acp";
import { parseACPTranscript } from "./acpTranscript";

describe("parseACPTranscript", () => {
	it("pairs tool results across messages while preserving assistant activity order", () => {
		expect(parseACPTranscript(MockACPTranscript)).toEqual([
			{
				type: "thinking",
				text: "I will compare the response with the expected value.",
			},
			{
				type: "response",
				text: "I found a mismatch in the expected response.",
			},
			{
				type: "tool",
				tool: {
					id: "run-tests",
					name: "Run unit tests",
					args: { command: "go test ./agent/..." },
					result: "ok github.com/coder/coder/v2/agent",
					status: "completed",
					isError: false,
				},
			},
			{
				type: "response",
				text: "Updated the response expectation. All affected tests pass.",
			},
		]);
	});

	it.each([
		{ status: "pending", isError: false, expected: "running" },
		{ status: "in_progress", isError: false, expected: "running" },
		{ status: "completed", isError: false, expected: "completed" },
		{ status: "failed", isError: false, expected: "error" },
		{ status: "completed", isError: true, expected: "error" },
	])(
		"normalizes $status and is_error=$isError",
		({ status, isError, expected }) => {
			const blocks = parseACPTranscript([
				{
					role: "assistant",
					content: [
						{
							type: "tool-call",
							tool_call_id: "tool",
							tool_name: "Read file",
							args: { path: "main.go" },
						},
					],
				},
				{
					role: "tool",
					content: [
						{
							type: "tool-result",
							tool_call_id: "tool",
							is_error: isError,
							result: JSON.stringify({ status, output: { lines: 10 } }),
						},
					],
				},
			]);
			expect(blocks).toEqual([
				{
					type: "tool",
					tool: {
						id: "tool",
						name: "Read file",
						args: { path: "main.go" },
						result: { lines: 10 },
						status: expected,
						isError: expected === "error",
					},
				},
			]);
		},
	);
});
