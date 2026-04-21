import { describe, expect, it } from "vitest";
import { agentsScrollRestorationKey } from "./router";

describe("agentsScrollRestorationKey", () => {
	it("groups all /agents routes under one restoration key", () => {
		expect(agentsScrollRestorationKey({ pathname: "/agents" })).toBe("/agents");
		expect(agentsScrollRestorationKey({ pathname: "/agents/chat-123" })).toBe(
			"/agents",
		);
		expect(
			agentsScrollRestorationKey({ pathname: "/agents/chat-123/settings" }),
		).toBe("/agents");
	});

	it("keeps non-agents routes isolated", () => {
		expect(agentsScrollRestorationKey({ pathname: "/workspaces" })).toBe(
			"/workspaces",
		);
		expect(agentsScrollRestorationKey({ pathname: "/login" })).toBe("/login");
	});
});
