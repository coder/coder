import { describe, expect, it } from "vitest";
import {
	buildAgentChatPath,
	readNewChildChatLocationState,
	safeBuildAgentChatPath,
} from "./navigation";

describe("buildAgentChatPath", () => {
	it("encodes chat IDs as a path segment", () => {
		expect(buildAgentChatPath({ chatId: "chat/id" })).toBe("/agents/chat%2Fid");
	});
});

describe("safeBuildAgentChatPath", () => {
	it("returns a path for a safe chat ID", () => {
		expect(safeBuildAgentChatPath({ chatId: "child-chat-id" })).toBe(
			"/agents/child-chat-id",
		);
	});

	it("recovers a leading safe segment from a malformed chat ID", () => {
		expect(
			safeBuildAgentChatPath({
				chatId: ["8f3a6131-1ce8-46f5-9", "b", "a8-4a36-beb2? no"].join(""),
			}),
		).toBe(["/agents/8f3a6131-1ce8-46f5-9", "b", "a8-4a36-beb2"].join(""));
	});

	it("returns null when no safe chat ID is recoverable", () => {
		expect(safeBuildAgentChatPath({ chatId: "? no" })).toBeNull();
		expect(safeBuildAgentChatPath({ chatId: "chat/id" })).toBeNull();
	});
});

describe("readNewChildChatLocationState", () => {
	it("returns the parent when every field is a string", () => {
		expect(
			readNewChildChatLocationState({
				parentChatId: "parent-1",
				parentChatTitle: "Parent",
				parentOrganizationId: "org-1",
				extra: true,
			}),
		).toEqual({
			parentChatId: "parent-1",
			parentChatTitle: "Parent",
			parentOrganizationId: "org-1",
		});
	});

	it("ignores missing, partial, or non-object state", () => {
		expect(readNewChildChatLocationState(null)).toBeUndefined();
		expect(readNewChildChatLocationState("parent-1")).toBeUndefined();
		expect(
			readNewChildChatLocationState({ parentChatId: "parent-1" }),
		).toBeUndefined();
	});
});
