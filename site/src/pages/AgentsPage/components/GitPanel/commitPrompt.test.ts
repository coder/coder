import { describe, expect, it, vi } from "vitest";
import type { ChatMessageInputRef } from "../AgentChatInput";
import { insertCommitPrompt } from "./commitPrompt";

const repoRoot = "/home/coder/coder";
const prompt = `Commit and push the working changes in ${repoRoot}. If there are unstaged files, commit them too.`;

function mockComposer(value = ""): ChatMessageInputRef {
	let current = value;
	return {
		getValue: () => current,
		insertText: vi.fn((text: string) => {
			current += text;
		}),
		setValue: vi.fn(),
		clear: vi.fn(),
		focus: vi.fn(),
		addFileReference: vi.fn(),
		getContentParts: vi.fn(() => []),
	};
}

describe("insertCommitPrompt", () => {
	it("inserts the prompt into an empty composer", () => {
		const input = mockComposer();
		insertCommitPrompt(input, repoRoot);

		expect(input.insertText).toHaveBeenCalledWith(prompt);
		expect(input.focus).toHaveBeenCalled();
	});

	it("treats a whitespace-only composer as empty", () => {
		const input = mockComposer("  \n");
		insertCommitPrompt(input, repoRoot);

		expect(input.insertText).toHaveBeenCalledWith(prompt);
	});

	it("separates the prompt from existing composer text", () => {
		const input = mockComposer("please review");
		insertCommitPrompt(input, repoRoot);

		expect(input.insertText).toHaveBeenCalledWith(`\n\n${prompt}`);
	});

	it("does not duplicate a prompt already in the composer", () => {
		const input = mockComposer(prompt);
		insertCommitPrompt(input, repoRoot);

		expect(input.insertText).not.toHaveBeenCalled();
		expect(input.focus).toHaveBeenCalled();
	});
});
