import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import {
	type ChatOverview,
	ChatOverviewCardView,
	hasOverviewContent,
	type OverviewTabId,
} from "./ChatOverviewCard";

const emptyOverview: ChatOverview = {
	isSubagent: false,
	repos: [],
	pullRequests: [],
	subagents: [],
	attachments: [],
};

const fullOverview: ChatOverview = {
	...emptyOverview,
	cost: { totalMicros: 1_250_000, unpricedRequests: 0 },
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

const renderCard = (overview: ChatOverview) => {
	const onOpenTab = vi.fn<(tabId: OverviewTabId) => void>();
	const router = createMemoryRouter(
		[
			{
				path: "/agents/:agentId",
				element: (
					<ChatOverviewCardView overview={overview} onOpenTab={onOpenTab} />
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
		const { onOpenTab } = renderCard(fullOverview);

		await user.click(screen.getByRole("button", { name }));

		expect(onOpenTab).toHaveBeenCalledExactlyOnceWith(tab);
	});

	it("opens the child chat from a sub-agent row", async () => {
		const user = userEvent.setup();
		const { onOpenTab, router } = renderCard(fullOverview);

		await user.click(screen.getByRole("link", { name: /Find the flake/ }));

		expect(router.state.location.pathname).toBe("/agents/child-1");
		expect(onOpenTab).not.toHaveBeenCalled();
	});

	it("shows every row after a click on the more row", async () => {
		const user = userEvent.setup();
		const repos = ["a", "b", "c", "d", "e"].map((name) => ({
			root: `/${name}`,
			branch: `branch-${name}`,
			additions: 1,
			deletions: 1,
		}));
		renderCard({ ...emptyOverview, repos });

		expect(screen.getAllByRole("button", { name: /branch-/ })).toHaveLength(3);
		await user.click(screen.getByRole("button", { name: "2 more" }));

		expect(screen.getAllByRole("button", { name: /branch-/ })).toHaveLength(5);
	});
});

describe("hasOverviewContent", () => {
	it("is false when every section is empty", () => {
		expect(hasOverviewContent(emptyOverview)).toBe(false);
	});

	it("is true with a single sub-agent", () => {
		expect(
			hasOverviewContent({
				...emptyOverview,
				subagents: [{ id: "c", title: "", status: "waiting" }],
			}),
		).toBe(true);
	});
});
