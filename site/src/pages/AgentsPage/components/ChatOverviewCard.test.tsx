import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useRef } from "react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import {
	type ChatOverview,
	ChatOverviewCardView,
	OVERVIEW_CARD_PINNED_KEY,
	type OverviewTabId,
	useOverviewCard,
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
	onOpenTab: (tabId: OverviewTabId) => void;
};

/** The top bar toggle and the card, wired as the chat page wires them. */
const OverviewHarness: React.FC<OverviewHarnessProps> = ({ onOpenTab }) => {
	const cardRef = useRef<HTMLElement>(null);
	const toggleRef = useRef<HTMLButtonElement>(null);
	const card = useOverviewCard({ cardRef, toggleRef });
	return (
		<>
			<button
				ref={toggleRef}
				type="button"
				aria-expanded={card.isOpen}
				onClick={card.toggle}
			>
				Toggle overview
			</button>
			<p>Transcript</p>
			{card.isOpen && (
				<ChatOverviewCardView
					ref={cardRef}
					overview={mockChatOverview}
					isPinned={card.isPinned}
					onPinnedChange={card.setPinned}
					onOpenTab={(tabId) => {
						card.dismiss();
						onOpenTab(tabId);
					}}
				/>
			)}
		</>
	);
};

const renderOverview = () => {
	const onOpenTab = vi.fn<(tabId: OverviewTabId) => void>();
	const router = createMemoryRouter(
		[
			{
				path: "/agents/:agentId",
				element: <OverviewHarness onOpenTab={onOpenTab} />,
			},
		],
		{ initialEntries: ["/agents/root"] },
	);
	const view = render(<RouterProvider router={router} />);
	return { onOpenTab, router, view };
};

const queryCard = () =>
	screen.queryByRole("complementary", { name: "Overview" });

const openCard = async (user: ReturnType<typeof userEvent.setup>) => {
	await user.click(screen.getByRole("button", { name: "Toggle overview" }));
	expect(queryCard()).toBeInTheDocument();
};

describe("ChatOverviewCard", () => {
	beforeEach(() => {
		localStorage.removeItem(OVERVIEW_CARD_PINNED_KEY);
	});

	it.each<{ name: RegExp; tab: OverviewTabId }>([
		{ name: /Cost/, tab: "summary" },
		{ name: /ci\/queue-only-jobs/, tab: "git" },
		{ name: /#123 Queue jobs/, tab: "git" },
		{ name: /plan\.html/, tab: "summary" },
	])(
		"opens the $tab tab from the row named $name and closes",
		async ({ name, tab }) => {
			const user = userEvent.setup();
			const { onOpenTab } = renderOverview();
			await openCard(user);

			await user.click(screen.getByRole("button", { name }));

			expect(onOpenTab).toHaveBeenCalledExactlyOnceWith(tab);
			expect(queryCard()).not.toBeInTheDocument();
		},
	);

	it("opens the child chat from a subagent row", async () => {
		const user = userEvent.setup();
		const { onOpenTab, router } = renderOverview();
		await openCard(user);

		await user.click(screen.getByRole("link", { name: /Find the flake/ }));

		expect(router.state.location.pathname).toBe("/agents/child-1");
		expect(onOpenTab).not.toHaveBeenCalled();
	});

	it("closes on the toggle, an outside click, and Escape", async () => {
		const user = userEvent.setup();
		renderOverview();
		const toggle = screen.getByRole("button", { name: "Toggle overview" });

		await openCard(user);
		await user.click(toggle);
		expect(queryCard()).not.toBeInTheDocument();

		await openCard(user);
		await user.click(screen.getByText("Transcript"));
		expect(queryCard()).not.toBeInTheDocument();

		await openCard(user);
		await user.click(screen.getByRole("button", { name: /Cost/ }));
		await openCard(user);
		screen.getByRole("button", { name: "Pin overview" }).focus();
		await user.keyboard("{Escape}");
		expect(queryCard()).not.toBeInTheDocument();
		expect(toggle).toHaveFocus();
	});

	it("stays open while pinned, in the next chat too, until the toggle closes it", async () => {
		const user = userEvent.setup();
		const { view } = renderOverview();
		await openCard(user);

		await user.click(screen.getByRole("button", { name: "Pin overview" }));
		await user.click(screen.getByText("Transcript"));
		await user.click(screen.getByRole("button", { name: /Cost/ }));
		expect(queryCard()).toBeInTheDocument();

		// A new chat mounts the page view again.
		view.unmount();
		renderOverview();
		expect(queryCard()).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: "Pin overview" }),
		).toHaveAttribute("aria-pressed", "true");

		await user.click(screen.getByRole("button", { name: "Toggle overview" }));
		expect(queryCard()).not.toBeInTheDocument();
		expect(localStorage.getItem(OVERVIEW_CARD_PINNED_KEY)).toBe("false");
	});
});
