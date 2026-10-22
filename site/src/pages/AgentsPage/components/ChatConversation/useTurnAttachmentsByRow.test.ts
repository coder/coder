import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { parseMessagesWithMergedTools } from "./messageParsing";
import { MockUnansweredAttachmentMessages } from "./storyFixtures";
import { useTurnAttachmentsByRow } from "./useTurnAttachmentsByRow";

describe("useTurnAttachmentsByRow", () => {
	it("adds the newest turn's entry once the chat stops", () => {
		const parsedMessages = parseMessagesWithMergedTools(
			MockUnansweredAttachmentMessages,
		);
		const { result, rerender } = renderHook(
			(isChatActive: boolean) =>
				useTurnAttachmentsByRow(parsedMessages, {
					hidesAttachmentRows: true,
					isChatActive,
					isCompacting: false,
					hasMoreMessages: false,
				}),
			{ initialProps: true },
		);
		expect([...result.current.keys()]).toEqual(["message:9"]);

		rerender(false);

		expect([...result.current.keys()]).toEqual(["message:9", "message:20"]);
	});
});
