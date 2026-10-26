import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import {
	type ChatOverview,
	ChatOverviewCardView,
	type OverviewTabId,
} from "./ChatOverviewCard";

const mockChatOverview: ChatOverview = {
	cost: { totalMicros: 1_250_000, unpricedRequests: 0 },
	isSubagent: false,
	repos: [
		{
			root: "/repo",
			branch: "ci/queue-only-jobs",
			additions: 82,
			deletions: 33,
		},
	],
	pullRequests: [{ ...MockChatDiffStatus, pull_request_title: "Queue jobs" }],
	subagents: [{ id: "child-1", title: "Find the flake", status: "running" }],
	attachments: [
		{
			key: "1:0",
			file: { type: "file", media_type: "text/html", name: "plan.html" },
			href: "/files/plan.html",
		},
	],
};

const renderCard = () => {
	const onOpenTab = vi.fn<(tabId: OverviewTabId) => void>();
	const router = createMemoryRouter(
		[
			{
				path: "/agents/:agentId",
				element: (
					<ChatOverviewCardView
						overview={mockChatOverview}
						onOpenTab={onOpenTab}
					/>
				),
			},
		],
		{ initialEntries: ["/agents/root"] },
	);
	render(<RouterProvider router={router} />);
	return { onOpenTab, router };
};

describe("ChatOverviewCardView", () => {
	it.each<{ name: RegExp; tab: OverviewTabId }>([
		{ name: /Cost/, tab: "summary" },
		{ name: /ci\/queue-only-jobs/, tab: "git" },
		{ name: /#123 Queue jobs/, tab: "git" },
		{ name: /plan\.html/, tab: "summary" },
	])("opens the $tab tab from the row named $name", async ({ name, tab }) => {
		const user = userEvent.setup();
		const { onOpenTab } = renderCard();

		await user.click(screen.getByRole("button", { name }));

		expect(onOpenTab).toHaveBeenCalledExactlyOnceWith(tab);
	});

	it("opens the child chat from a subagent row", async () => {
		const user = userEvent.setup();
		const { onOpenTab, router } = renderCard();

		await user.click(screen.getByRole("link", { name: /Find the flake/ }));

		expect(router.state.location.pathname).toBe("/agents/child-1");
		expect(onOpenTab).not.toHaveBeenCalled();
	});
});
