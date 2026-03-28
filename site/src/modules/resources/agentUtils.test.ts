import {
	MockWorkspaceAgent,
	MockWorkspaceSubAgent,
} from "#/testHelpers/entities";
import {
	CHAT_AGENT_SUFFIX,
	getVisibleWorkspaceAgents,
	isChatAgent,
} from "./agentUtils";

describe("agentUtils", () => {
	it("matches the chat agent suffix case-insensitively", () => {
		expect(
			isChatAgent({
				name: `workspace${CHAT_AGENT_SUFFIX.toUpperCase()}`,
			}),
		).toBe(true);
	});

	it("hides chat agents and their descendants", () => {
		const chatAgent = {
			...MockWorkspaceAgent,
			id: "chat-agent",
			name: `workspace${CHAT_AGENT_SUFFIX}`,
		};
		const chatSubAgent = {
			...MockWorkspaceSubAgent,
			id: "chat-sub-agent",
			name: "workspace-child-agent",
			parent_id: chatAgent.id,
		};
		const visibleAgent = {
			...MockWorkspaceAgent,
			id: "visible-agent",
			name: "workspace-agent",
		};

		expect(
			getVisibleWorkspaceAgents([chatAgent, chatSubAgent, visibleAgent]),
		).toEqual([visibleAgent]);
	});
});
