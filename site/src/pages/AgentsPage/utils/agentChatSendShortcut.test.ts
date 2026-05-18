import { describe, expect, it } from "vitest";
import {
	DEFAULT_AGENT_CHAT_SEND_SHORTCUT,
	getAgentChatSendShortcut,
	MODIFIER_AGENT_CHAT_SEND_SHORTCUT,
} from "./agentChatSendShortcut";

describe("getAgentChatSendShortcut", () => {
	it("returns the stored shortcut when present", () => {
		expect(
			getAgentChatSendShortcut(MODIFIER_AGENT_CHAT_SEND_SHORTCUT, false),
		).toBe(MODIFIER_AGENT_CHAT_SEND_SHORTCUT);
	});

	it("uses the modifier shortcut while preferences are loading", () => {
		expect(getAgentChatSendShortcut(undefined, true)).toBe(
			MODIFIER_AGENT_CHAT_SEND_SHORTCUT,
		);
	});

	it("uses the default shortcut after preferences finish loading", () => {
		expect(getAgentChatSendShortcut(undefined, false)).toBe(
			DEFAULT_AGENT_CHAT_SEND_SHORTCUT,
		);
	});
});
