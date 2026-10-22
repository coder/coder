import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { parseMessagesWithMergedTools } from "./messageParsing";
import { MockUnansweredAttachmentMessages } from "./storyFixtures";
import { useNewestTurnAttachments } from "./useNewestTurnAttachments";

describe("useNewestTurnAttachments", () => {
	it("hosts the newest turn's pill once the chat stops", () => {
		const parsedMessages = parseMessagesWithMergedTools(
			MockUnansweredAttachmentMessages,
		);
		const { result, rerender } = renderHook(
			(isChatActive: boolean) =>
				useNewestTurnAttachments(parsedMessages, {
					hidesAttachmentRows: true,
					isChatActive,
					isNextTurnStarting: false,
					isCompacting: false,
					hasMoreMessages: false,
				}),
			{ initialProps: true },
		);
		expect(result.current).toBeUndefined();

		rerender(false);

		expect(result.current?.rowKey).toBe("message:20");
	});
});
