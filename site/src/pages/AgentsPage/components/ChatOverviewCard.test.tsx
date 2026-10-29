import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, RouterProvider } from "react-router";
import type { ChatStatus } from "#/api/typesGenerated";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import {
	OVERVIEW_CARD_OPEN_KEY,
	useOverviewCard,
} from "../hooks/useOverviewCard";
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
	pullRequests: [
		{
			number: "123",
			status: { ...MockChatDiffStatus, pull_request_title: "Queue jobs" },
		},
	],
	subagents: [{ id: "child-1", title: "Find the flake", status: "running" }],
	attachments: [
		{
			key: "1:0",
			file: { type: "file", media_type: "text/html", name: "plan.html" },
			href: "/files/plan.html",
		},
	],
};

type OverviewHarnessProps = {
	overview: ChatOverview;
	onOpenTab: (tabId: OverviewTabId) => void;
};

/**
 * The hook and the card, wired as the chat page wires them. The button
 * stands in for the top bar toggle, so this covers the open state, not
 * ChatTopBar.
 */
const OverviewHarness: React.FC<OverviewHarnessProps> = ({
	overview,
	onOpenTab,
}) => {
	const card = useOverviewCard();
	return (
		<>
			<button type="button" aria-expanded={card.isOpen} onClick={card.toggle}>
				Toggle overview
			</button>
			<p>Transcript</p>
			{card.isOpen && (
				<ChatOverviewCardView overview={overview} onOpenTab={onOpenTab} />
			)}
		</>
	);
};

const renderOverview = (overview = mockChatOverview) => {
	const onOpenTab = vi.fn<(tabId: OverviewTabId) => void>();
	const router = createMemoryRouter(
		[
			{
				path: "/agents/:agentId",
				element: <OverviewHarness overview={overview} onOpenTab={onOpenTab} />,
			},
		],
		{ initialEntries: ["/agents/root"] },
	);
	const view = render(<RouterProvider router={router} />);
	return { onOpenTab, router, view };
};

const expectOpen = (expanded: boolean) =>
	screen.getByRole("button", { name: "Toggle overview", expanded });

const openCard = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("button", { name: "Toggle overview" }));
	expectOpen(true);
};

describe("ChatOverviewCard", () => {
	beforeEach(() => {
		localStorage.removeItem(OVERVIEW_CARD_OPEN_KEY);
	});

	it.each<{ name: RegExp; tab: OverviewTabId }>([
		{ name: /Cost/, tab: "summary" },
		{ name: /ci\/queue-only-jobs/, tab: "git" },
		{ name: /#123 Queue jobs/, tab: "git" },
		{ name: /plan\.html/, tab: "summary" },
	])("opens the $tab tab from the row named $name", async ({ name, tab }) => {
		const user = userEvent.setup();
		const { onOpenTab } = renderOverview();
		await openCard(user);

		await user.click(screen.getByRole("button", { name }));

		expect(onOpenTab).toHaveBeenCalledExactlyOnceWith(tab);
	});

	it("opens the child chat from a subagent row", async () => {
		const user = userEvent.setup();
		const { onOpenTab, router } = renderOverview();
		await openCard(user);

		await user.click(screen.getByRole("link", { name: /Find the flake/ }));

		expect(router.state.location.pathname).toBe("/agents/child-1");
		expect(onOpenTab).not.toHaveBeenCalled();
	});

	it("lists at most 5 active subagents and counts all of them", async () => {
		const user = userEvent.setup();
		const statuses: ChatStatus[] = [
			"waiting",
			"running",
			"error",
			"requires_action",
			"waiting",
			"interrupting",
			"running",
			"running",
		];
		renderOverview({
			...mockChatOverview,
			subagents: statuses.map((status, index) => ({
				id: `child-${index}`,
				title: `Subagent ${index}`,
				status,
			})),
		});
		await openCard(user);

		const section = screen.getByRole("region", {
			name: "Subagents · 6 of 8 active",
		});
		const links = within(section).getAllByRole("link");
		expect(links.map((link) => link.textContent)).toEqual(
			[1, 2, 3, 5, 6].map((index) => `Subagent ${index}`),
		);
		expect(within(section).queryByRole("button")).not.toBeInTheDocument();
	});

	it("stays open, in the next chat too, until the toggle closes it", async () => {
		const user = userEvent.setup();
		const { view } = renderOverview();
		await openCard(user);

		await user.click(screen.getByText("Transcript"));
		await user.click(screen.getByRole("button", { name: /Cost/ }));
		await user.keyboard("{Escape}");
		expectOpen(true);

		// A new chat mounts the page view again.
		view.unmount();
		renderOverview();
		expectOpen(true);

		await user.click(screen.getByRole("button", { name: "Toggle overview" }));
		expectOpen(false);
		expect(localStorage.getItem(OVERVIEW_CARD_OPEN_KEY)).toBe("false");
	});
});
