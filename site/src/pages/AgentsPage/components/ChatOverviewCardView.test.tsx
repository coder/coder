import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ChatStatus } from "#/api/typesGenerated";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import { render } from "#/testHelpers/renderHelpers";
import {
	type ChatOverview,
	ChatOverviewCardView,
	type OverviewTarget,
} from "./ChatOverviewCardView";

const mockChatOverview: ChatOverview = {
	cost: { total_cost_micros: 1_250_000, unpriced_request_count: 0 },
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
			role: "assistant",
			file: { type: "file", media_type: "text/html", name: "plan.html" },
			href: "/files/plan.html",
		},
	],
};

const renderCard = (overview = mockChatOverview) => {
	const onOpenTarget = vi.fn<(target: OverviewTarget) => void>();
	const { router } = render(
		<ChatOverviewCardView overview={overview} onOpenTarget={onOpenTarget} />,
	);
	return { onOpenTarget, router };
};

const withSubagentStatuses = (statuses: ChatStatus[]): ChatOverview => ({
	...mockChatOverview,
	subagents: statuses.map((status, index) => ({
		id: `child-${index}`,
		title: `Subagent ${index}`,
		status,
	})),
});

describe("ChatOverviewCardView", () => {
	it.each<{ name: RegExp; tab: OverviewTarget["tabId"] }>([
		{ name: /Cost/, tab: "summary" },
		{ name: /ci\/queue-only-jobs/, tab: "git" },
		{ name: /#123 Queue jobs/, tab: "git" },
		{ name: /plan\.html/, tab: "summary" },
	])("opens the $tab tab from the row named $name", async ({ name, tab }) => {
		const user = userEvent.setup();
		const { onOpenTarget } = renderCard();

		await user.click(screen.getByRole("button", { name }));

		expect(onOpenTarget).toHaveBeenCalledExactlyOnceWith({ tabId: tab });
	});

	it("opens nothing from a repository without changes", async () => {
		const user = userEvent.setup();
		const { onOpenTarget } = renderCard({
			...mockChatOverview,
			repos: [{ root: "/clean", branch: "main", additions: 0, deletions: 0 }],
		});

		await user.click(screen.getByText("main"));

		expect(screen.queryByRole("button", { name: /main/ })).toBeNull();
		expect(onOpenTarget).not.toHaveBeenCalled();
	});

	it("opens the child chat from a subagent row", async () => {
		const user = userEvent.setup();
		const { onOpenTarget, router } = renderCard();

		await user.click(screen.getByRole("link", { name: /Find the flake/ }));

		expect(router.state.location.pathname).toBe("/agents/child-1");
		expect(onOpenTarget).not.toHaveBeenCalled();
	});

	it("lists at most 5 non-idle subagents, the ones that need the user first", () => {
		renderCard(
			withSubagentStatuses([
				"running",
				"waiting",
				"running",
				"interrupting",
				"running",
				"error",
				"waiting",
				"requires_action",
			]),
		);

		const section = screen.getByRole("region", {
			name: "Subagents · 6 of 8 not idle",
		});
		const links = within(section).getAllByRole("link");
		expect(links.map((link) => link.textContent)).toEqual(
			[5, 7, 0, 2, 3].map((index) => `Subagent ${index}`),
		);
	});

	it("counts idle subagents instead of showing the empty state", () => {
		renderCard(withSubagentStatuses(["waiting", "waiting", "waiting"]));

		expect(
			screen.getByRole("region", { name: "Subagents · 0 of 3 not idle" }),
		).toHaveAccessibleName();
	});

	it("shows a row's full label in a tooltip on hover", async () => {
		const user = userEvent.setup();
		renderCard();

		const row = screen.getByRole("button", { name: /#123 Queue jobs/ });
		await user.hover(row);

		await waitFor(() =>
			expect(row).toHaveAccessibleDescription("#123 Queue jobs"),
		);
	});

	it("opens the tooltip of a repository without changes from the keyboard", async () => {
		const user = userEvent.setup();
		renderCard({
			...mockChatOverview,
			cost: undefined,
			repos: [{ root: "/clean", branch: "main", additions: 0, deletions: 0 }],
		});

		await user.tab();

		await waitFor(() =>
			expect(document.activeElement).toHaveAccessibleDescription("main"),
		);
	});

	it("opens a subagent row's tooltip from the keyboard", async () => {
		renderCard({ ...mockChatOverview, cost: undefined, repos: [] });

		const row = screen.getByRole("link", { name: /Find the flake/ });
		act(() => row.focus());

		await waitFor(() =>
			expect(row).toHaveAccessibleDescription("Find the flake"),
		);
	});
});
