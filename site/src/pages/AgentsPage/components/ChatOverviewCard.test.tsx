import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ChatStatus } from "#/api/typesGenerated";
import { MockChatDiffStatus, mockChatCost } from "#/testHelpers/chatEntities";
import { render } from "#/testHelpers/renderHelpers";
import {
	type ChatOverview,
	ChatOverviewCardView,
	getOverviewCost,
	type OverviewTarget,
} from "./ChatOverviewCard";
import { viewIdFor } from "./GitPanel/GitPanel";

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
	const onOpenTab = vi.fn<(target: OverviewTarget) => void>();
	const { router } = render(
		<ChatOverviewCardView overview={overview} onOpenTab={onOpenTab} />,
	);
	return { onOpenTab, router };
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
	it.each<{ name: RegExp; target: OverviewTarget }>([
		{ name: /Cost/, target: { tabId: "summary" } },
		{
			name: /ci\/queue-only-jobs/,
			target: { tabId: "git", view: { type: "local", repoRoot: "/repo" } },
		},
		{
			name: /#123 Queue jobs/,
			target: {
				tabId: "git",
				view: { type: "remote", refId: viewIdFor(MockChatDiffStatus) },
			},
		},
		{ name: /plan\.html/, target: { tabId: "summary" } },
	])(
		"opens $target.tabId from the row named $name",
		async ({ name, target }) => {
			const user = userEvent.setup();
			const { onOpenTab } = renderCard();

			await user.click(screen.getByRole("button", { name }));

			expect(onOpenTab).toHaveBeenCalledExactlyOnceWith(target);
		},
	);

	it("opens the child chat from a subagent row", async () => {
		const user = userEvent.setup();
		const { onOpenTab, router } = renderCard();

		await user.click(screen.getByRole("link", { name: /Find the flake/ }));

		expect(router.state.location.pathname).toBe("/agents/child-1");
		expect(onOpenTab).not.toHaveBeenCalled();
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
});

describe("getOverviewCost", () => {
	const cost = mockChatCost("root", 1_250_000);

	it.each([
		{
			name: "AI Gateway is off",
			showCost: false,
			data: cost,
			isError: false,
			expected: undefined,
		},
		{
			name: "the request is pending",
			showCost: true,
			data: undefined,
			isError: false,
			expected: undefined,
		},
		{
			name: "the request failed",
			showCost: true,
			data: undefined,
			isError: true,
			expected: "unavailable",
		},
		{
			name: "a refetch failed",
			showCost: true,
			data: cost,
			isError: true,
			expected: cost,
		},
		{
			name: "the chat has no requests",
			showCost: true,
			data: { ...cost, request_count: 0 },
			isError: false,
			expected: undefined,
		},
	])(
		"returns $expected when $name",
		({ showCost, data, isError, expected }) => {
			expect(getOverviewCost(showCost, { data, isError })).toEqual(expected);
		},
	);
});
