import { describe, expect, it } from "vitest";
import { mergeStreamPayload, parseStreamingJSON } from "./streamingJson";

describe("parseStreamingJSON", () => {
	it("parses complete JSON objects", () => {
		expect(parseStreamingJSON('{"ok":true,"count":2}')).toEqual({
			ok: true,
			count: 2,
		});
	});

	it("parses partial objects with an in-progress string value", () => {
		expect(parseStreamingJSON('{"command":"git che')).toEqual({
			command: "git che",
		});
	});

	it("returns parsed fields for partial objects with trailing incomplete field", () => {
		expect(parseStreamingJSON('{"a":1,"b":')).toEqual({ a: 1 });
	});

	it("returns null when content is not JSON-like", () => {
		expect(parseStreamingJSON("hello world")).toBeNull();
	});
});

describe("mergeStreamPayload", () => {
	it("prefers explicit non-string values", () => {
		expect(
			mergeStreamPayload(undefined, undefined, { done: true }, undefined),
		).toEqual({
			value: { done: true },
		});
	});

	it("parses explicit string values and preserves raw text", () => {
		expect(
			mergeStreamPayload(undefined, undefined, '{"output":"ok"}', undefined),
		).toEqual({
			value: { output: "ok" },
			rawText: '{"output":"ok"}',
		});
	});

	it("merges incoming deltas into parsed partial JSON", () => {
		const first = mergeStreamPayload(
			undefined,
			undefined,
			undefined,
			'{"command":"git ',
		);
		expect(first).toEqual({
			value: { command: "git" },
			rawText: '{"command":"git ',
		});

		const second = mergeStreamPayload(
			first.value,
			first.rawText,
			undefined,
			'status"}',
		);
		expect(second).toEqual({
			value: { command: "git status" },
			rawText: '{"command":"git status"}',
		});
	});

	it("keeps structured existing values when only deltas arrive", () => {
		expect(
			mergeStreamPayload({ a: 1 }, undefined, undefined, "ignored"),
		).toEqual({
			value: { a: 1 },
		});
	});

	it("returns existing payload when delta is empty", () => {
		expect(mergeStreamPayload("abc", "abc", undefined, undefined)).toEqual({
			value: "abc",
			rawText: "abc",
		});
	});
});
