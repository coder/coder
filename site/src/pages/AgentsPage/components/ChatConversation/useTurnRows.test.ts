import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { parseMessagesWithMergedTools } from "./messageParsing";
import { MockUnansweredAttachmentMessages } from "./storyFixtures";
import { useTurnRows } from "./useTurnRows";

describe("useTurnRows", () => {
	it("adds the newest turn's entry once the chat stops", () => {
		const parsedMessages = parseMessagesWithMergedTools(
			MockUnansweredAttachmentMessages,
		);
		const { result, rerender } = renderHook(
			(isChatActive: boolean) =>
				useTurnRows(parsedMessages, {
					hasLiveAssistant: false,
					hidesAttachmentRows: true,
					isChatActive,
					isNextTurnStarting: false,
					isCompacting: false,
					hasMoreMessages: false,
				}),
			{ initialProps: true },
		);
		expect([...result.current[1].keys()]).toEqual(["message:9"]);

		rerender(false);

		expect([...result.current[1].keys()]).toEqual(["message:9", "message:20"]);
	});
});
