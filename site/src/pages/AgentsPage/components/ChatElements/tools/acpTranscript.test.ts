import { describe, expect, it } from "vitest";
import type {
	ChatACPTranscriptMessage,
	ChatMessagePart,
} from "#/api/typesGenerated";
import { MockACPTranscript } from "#/testHelpers/acp";
import { mergeACPTranscriptDelta, parseACPTranscript } from "./acpTranscript";

const record = (
	message_index: number,
	role: ChatACPTranscriptMessage["role"],
	part: ChatMessagePart,
) => `${JSON.stringify({ message_index, role, part })}\n`;

describe("mergeACPTranscriptDelta", () => {
	it("appends text and reasoning chunks across batched and fragmented records", () => {
		const delta = [
			record(0, "assistant", { type: "reasoning", text: "Thinking" }),
			record(0, "assistant", { type: "reasoning", text: " carefully" }),
			record(0, "assistant", { type: "text", text: 'A "quoted" line\n雪' }),
			record(0, "assistant", { type: "text", text: " and more" }),
		].join("");
		const expected = {
			messages: [
				{
					role: "assistant",
					content: [
						{ type: "reasoning", text: "Thinking carefully" },
						{ type: "text", text: 'A "quoted" line\n雪 and more' },
					],
				},
			],
		};
		expect(mergeACPTranscriptDelta(undefined, undefined, delta)).toEqual({
			value: expected,
			rawText: "",
		});
		let merged = mergeACPTranscriptDelta(undefined, undefined, "");
		for (const chunk of delta) {
			merged = mergeACPTranscriptDelta(merged.value, merged.rawText, chunk);
		}
		expect(merged).toEqual({ value: expected, rawText: "" });
	});

	it("preserves reserved message positions and replaces tool updates by ID immutably", () => {
		const call = MockACPTranscript[0].content[2];
		const result = MockACPTranscript[1].content[0];
		const initial = mergeACPTranscriptDelta(
			undefined,
			undefined,
			record(0, "assistant", call) +
				record(2, "assistant", MockACPTranscript[2].content[0]),
		);
		const before = structuredClone(initial);
		const updatedCall = {
			...call,
			tool_name: "Run agent tests",
			args: { command: "go test -race ./agent/..." },
		};
		const completed = mergeACPTranscriptDelta(
			initial.value,
			initial.rawText,
			record(0, "assistant", updatedCall) + record(1, "tool", result),
		);
		expect(initial).toEqual(before);
		expect(completed.value.messages).toEqual([
			{ role: "assistant", content: [updatedCall] },
			MockACPTranscript[1],
			MockACPTranscript[2],
		]);
		const failedResult = {
			...result,
			is_error: true,
			result: { status: "failed" },
		};
		const failed = mergeACPTranscriptDelta(
			completed.value,
			completed.rawText,
			record(1, "tool", failedResult),
		);
		expect(failed.value.messages[1].content).toEqual([failedResult]);
	});

	it("ignores malformed records without discarding valid activity or the unfinished tail", () => {
		const valid = record(0, "assistant", { type: "text", text: "Hello" });
		const invalid = [
			"invalid json",
			JSON.stringify({
				message_index: -1,
				role: "assistant",
				part: { type: "text", text: "bad index" },
			}),
			JSON.stringify({
				message_index: 0.5,
				role: "assistant",
				part: { type: "text", text: "bad index" },
			}),
			JSON.stringify({
				message_index: 0,
				role: "user",
				part: { type: "text", text: "hidden" },
			}),
			JSON.stringify({
				message_index: 0,
				role: "assistant",
				part: { type: "text", text: 123 },
			}),
		].join("\n");
		const merged = mergeACPTranscriptDelta(
			undefined,
			undefined,
			`${invalid}\n${valid}{"message_index":`,
		);
		expect(merged).toEqual({
			value: {
				messages: [
					{ role: "assistant", content: [{ type: "text", text: "Hello" }] },
				],
			},
			rawText: '{"message_index":',
		});
	});
});

describe("parseACPTranscript", () => {
	it("ignores invalid transcript data at the tool result boundary", () => {
		for (const messages of [
			undefined,
			null,
			{},
			"invalid",
			[null, { role: "assistant", content: null }],
		]) {
			expect(parseACPTranscript(messages)).toEqual([]);
		}
	});
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
