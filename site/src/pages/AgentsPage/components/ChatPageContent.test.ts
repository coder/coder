import { describe, expect, it } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { workspaceContextFromMessages } from "./ChatPageContent";

const now = "2026-05-08T00:00:00Z";

const message = (
	id: number,
	content: readonly TypesGen.ChatMessagePart[],
): TypesGen.ChatMessage => ({
	id,
	chat_id: "chat-1",
	created_at: now,
	role: "assistant",
	content,
});

const contextFile = (
	agentId: string,
	path: string,
): TypesGen.ChatContextFilePart => ({
	type: "context-file",
	context_file_path: path,
	context_file_agent_id: agentId,
});

const skill = (name: string, agentId: string): TypesGen.ChatSkillPart => ({
	type: "skill",
	skill_name: name,
	context_file_agent_id: agentId,
});

const summarizeContextPart = (part: TypesGen.ChatMessagePart): string => {
	if (part.type === "context-file") {
		return `context:${part.context_file_path}`;
	}
	if (part.type === "skill") {
		return `skill:${part.skill_name}`;
	}
	return part.type;
};

describe("workspaceContextFromMessages", () => {
	it("returns only the latest workspace context snapshot", () => {
		const context = workspaceContextFromMessages(
			[
				message(1, [
					contextFile("agent-1", "/old/AGENTS.md"),
					skill("old-skill", "agent-1"),
				]),
				message(2, [{ type: "text", text: "No context on this turn." }]),
				message(3, [
					contextFile("agent-1", "/new/AGENTS.md"),
					skill("new-skill", "agent-1"),
				]),
			],
			"agent-1",
		);

		expect(context?.map(summarizeContextPart)).toEqual([
			"context:/new/AGENTS.md",
			"skill:new-skill",
		]);
	});

	it("ignores stale context when the latest snapshot belongs to another agent", () => {
		const context = workspaceContextFromMessages(
			[
				message(1, [
					contextFile("agent-1", "/old/AGENTS.md"),
					skill("old-skill", "agent-1"),
				]),
				message(2, [
					contextFile("agent-2", "/new/AGENTS.md"),
					skill("new-skill", "agent-2"),
				]),
			],
			"agent-1",
		);

		expect(context).toBeUndefined();
	});
});
