import { describe, expect, it } from "vitest";
import { resolveSendChatMessageTargetId } from "./SendChatMessageTool";

const resultId = "5f2f9a3e-1d6c-4c1e-9d8e-2b6f4a1c7d10";
const argsId = "9c0e4d2a-7b3f-4e8a-b1d2-6f5a3c8e9b21";

describe("resolveSendChatMessageTargetId", () => {
	it("prefers the chat id from the result", () => {
		expect(resolveSendChatMessageTargetId(resultId, argsId, false)).toBe(
			resultId,
		);
		expect(resolveSendChatMessageTargetId(resultId, argsId, true)).toBe(
			resultId,
		);
	});

	it("falls back to the argument chat id for successful results", () => {
		expect(resolveSendChatMessageTargetId(undefined, argsId, false)).toBe(
			argsId,
		);
		expect(resolveSendChatMessageTargetId(undefined, "parent", false)).toBe(
			"parent",
		);
	});

	it("ignores a chat id argument for error results", () => {
		expect(
			resolveSendChatMessageTargetId(undefined, argsId, true),
		).toBeUndefined();
	});

	it("keeps the literal parent argument for error results", () => {
		expect(resolveSendChatMessageTargetId(undefined, "parent", true)).toBe(
			"parent",
		);
	});

	it("returns undefined when neither side names a chat", () => {
		expect(
			resolveSendChatMessageTargetId(undefined, undefined, false),
		).toBeUndefined();
		expect(resolveSendChatMessageTargetId("", "", false)).toBeUndefined();
	});
});
