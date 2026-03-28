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
	it("matches the chat agent suffix case-insensitively for root agents", () => {
		expect(
			isChatAgent({
				name: `workspace${CHAT_AGENT_SUFFIX.toUpperCase()}`,
				parent_id: null,
			}),
		).toBe(true);
		expect(
			isChatAgent({
				name: `workspace${CHAT_AGENT_SUFFIX}`,
				parent_id: "parent-agent",
			}),
		).toBe(false);
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

	it("keeps child agents with the chat suffix visible when their parent is not hidden", () => {
		const rootAgent = {
			...MockWorkspaceAgent,
			id: "root-agent",
			name: "workspace-agent",
		};
		const childSuffixAgent = {
			...MockWorkspaceSubAgent,
			id: "child-suffix-agent",
			name: `workspace-child${CHAT_AGENT_SUFFIX}`,
			parent_id: rootAgent.id,
		};

		expect(getVisibleWorkspaceAgents([rootAgent, childSuffixAgent])).toEqual([
			rootAgent,
			childSuffixAgent,
		]);
	});
});
