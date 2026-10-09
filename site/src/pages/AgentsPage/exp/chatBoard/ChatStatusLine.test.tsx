import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { shortRelativeTime } from "#/utils/time";
import { ChatStatusLine } from "./ChatStatusLine";

const pr = (number: number, state: "open" | "merged" = "open") => ({
	chat_id: MockChat.id,
	url: `https://github.com/coder/coder/pull/${number}`,
	pr_number: number,
	pull_request_state: state,
	pull_request_title: `Fix ${number}`,
	pull_request_draft: false,
	changes_requested: false,
	remote_origin: "https://github.com/coder/coder",
	git_branch: `fix-${number}`,
	additions: 12,
	deletions: 3,
	changed_files: 2,
});

const chat = (overrides: Partial<Chat>): Chat => ({
	...MockChat,
	last_turn_summary: "Fixed the build",
	diff_statuses: [pr(12)],
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

	it("opens a menu linking every PR when the chat has several", async () => {
		const user = userEvent.setup();
		render(
			<ChatStatusLine
				chat={chat({ diff_statuses: [pr(12), pr(13, "merged")] })}
			/>,
		);
		await user.click(screen.getByRole("button", { name: "2 pull requests" }));
		const links = await screen.findAllByRole("menuitem");
		expect(links.map((link) => link.getAttribute("href"))).toEqual([
			"https://github.com/coder/coder/pull/12",
			"https://github.com/coder/coder/pull/13",
		]);
	});
});
