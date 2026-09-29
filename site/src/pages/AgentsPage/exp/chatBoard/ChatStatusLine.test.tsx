import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat, MockChatDiffStatus } from "#/testHelpers/chatEntities";
import { shortRelativeTime } from "#/utils/time";
import { ChatStatusLine } from "./ChatStatusLine";

const chat = (overrides: Partial<Chat>): Chat => ({
	...MockChat,
	last_turn_summary: "Fixed the build",
	diff_statuses: [
		{
			chat_id: MockChat.id,
			url: "https://github.com/coder/coder/pull/12",
			pr_number: 12,
			pull_request_state: "open",
			pull_request_title: "Fix",
			pull_request_draft: false,
			changes_requested: false,
			additions: 12,
			deletions: 3,
			changed_files: 2,
		},
	],
	...overrides,
});

const lineText = (c: Chat) =>
	render(<ChatStatusLine chat={c} />).container.textContent ?? "";

describe("ChatStatusLine", () => {
	it("ends with the age when the chat is idle, and carries no diff counts", () => {
		const idle = chat({ status: "waiting", has_unread: true });
		const text = lineText(idle);

		expect(text.endsWith(shortRelativeTime(idle.updated_at))).toBe(true);
		expect(text).toContain("Fixed the build");
		expect(text).not.toContain("+12");
		expect(text).not.toContain("−3");
		expect(
			screen.getByRole("link", { name: "#12, Pull request open" }),
		).toHaveAttribute("href", "https://github.com/coder/coder/pull/12");
	});

	it("omits the age while the chat is working", () => {
		const running = chat({ status: "running" });
		const text = lineText(running);

		expect(text).not.toContain(shortRelativeTime(running.updated_at));
		expect(text.endsWith("Fixed the build")).toBe(true);
	});

	it("renders nothing for a working chat with no PR and no last turn", () => {
		expect(
			lineText(
				chat({
					status: "running",
					last_turn_summary: null,
					diff_statuses: [],
				}),
			),
		).toBe("");
	});

	it("links a badge for each PR", () => {
		render(
			<ChatStatusLine
				chat={chat({
					diff_statuses: [
						{
							...MockChatDiffStatus,
							git_branch: "feat/one",
							url: "https://github.com/coder/coder/pull/12",
							pr_number: 12,
						},
						{
							...MockChatDiffStatus,
							git_branch: "feat/two",
							url: "https://github.com/coder/coder/pull/13",
							pr_number: 13,
							pull_request_state: "merged",
						},
					],
				})}
			/>,
		);

		expect(
			screen.getByRole("link", { name: "#12, Pull request open" }),
		).toHaveAttribute("href", "https://github.com/coder/coder/pull/12");
		expect(
			screen.getByRole("link", { name: "#13, Pull request merged" }),
		).toHaveAttribute("href", "https://github.com/coder/coder/pull/13");
	});
});
