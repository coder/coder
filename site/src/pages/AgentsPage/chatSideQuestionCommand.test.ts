import { describe, expect, it } from "vitest";
import { parseChatSideQuestionCommand } from "./chatSideQuestionCommand";

describe("parseChatSideQuestionCommand", () => {
	it("detects side questions at the start of the trimmed prompt", () => {
		expect(parseChatSideQuestionCommand("  /btw what changed?")).toEqual({
			kind: "sideQuestion",
			question: "what changed?",
		});
	});

	it("rejects empty side questions", () => {
		expect(parseChatSideQuestionCommand("/btw")).toEqual({ kind: "invalid" });
		expect(parseChatSideQuestionCommand("/btw   ")).toEqual({
			kind: "invalid",
		});
	});

	it("treats mid-message commands as normal prompts", () => {
		expect(parseChatSideQuestionCommand("hello /btw what changed?")).toEqual({
			kind: "normal",
			prompt: "hello /btw what changed?",
		});
	});

	it("allows an escaped command to be sent as a normal prompt", () => {
		expect(parseChatSideQuestionCommand("//btw what changed?")).toEqual({
			kind: "normal",
			prompt: "/btw what changed?",
		});
	});
});
