import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ChatStatusLine } from "./ChatStatusLine";

const prChat: Chat = {
	...MockChat,
	diff_status: {
		chat_id: MockChat.id,
		url: "https://github.com/coder/coder/pull/123",
		pull_request_state: "open",
		pull_request_title: "Add a board",
		pull_request_draft: false,
		changes_requested: false,
		additions: 1,
		deletions: 0,
		changed_files: 1,
		pr_number: 123,
	},
};

describe("ChatStatusLine", () => {
	it("names the PR link by its visible number and state", () => {
		renderComponent(<ChatStatusLine chat={prChat} />);
		const link = screen.getByRole("link", { name: "#123, Pull request open" });
		expect(link).toHaveAttribute("href", prChat.diff_status?.url);
	});
});
