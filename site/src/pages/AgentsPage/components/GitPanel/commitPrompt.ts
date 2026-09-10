import type { ChatMessageInputRef } from "../AgentChatInput";

export function insertCommitPrompt(
	input: ChatMessageInputRef,
	repoRoot: string,
): void {
	const prompt = `Commit and push the working changes in ${repoRoot}. If there are unstaged files, commit them too.`;
	const current = input.getValue();
	if (!current.includes(prompt)) {
		input.insertText(current.trim() ? `\n\n${prompt}` : prompt);
	}
	input.focus();
}
