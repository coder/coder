import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { ChatDiffStatus } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { PRPills } from "./PRPills";

const pr = (
	number: number,
	state: "open" | "merged" = "open",
): ChatDiffStatus => ({
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

describe("PRPills", () => {
	it("gives one pill per state, each listing only its PRs", async () => {
		const user = userEvent.setup();
		render(<PRPills prs={[pr(12, "merged"), pr(13), pr(14, "merged")]} />);
		expect(
			screen.getAllByRole("button").map((b) => b.getAttribute("aria-label")),
		).toEqual(["1 open pull request", "2 merged pull requests"]);

		await user.click(
			screen.getByRole("button", { name: "2 merged pull requests" }),
		);
		const links = await screen.findAllByRole("menuitem");
		expect(links.map((link) => link.getAttribute("href"))).toEqual([
			"https://github.com/coder/coder/pull/12",
			"https://github.com/coder/coder/pull/14",
		]);
	});
});
