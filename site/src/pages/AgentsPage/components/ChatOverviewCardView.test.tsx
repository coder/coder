import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ChatStatus } from "#/api/typesGenerated";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import { render } from "#/testHelpers/renderHelpers";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
import {
	type ChatOverview,
	ChatOverviewCardView,
	type OverviewTarget,
} from "./ChatOverviewCardView";
import { viewIdFor } from "./GitPanel/gitViews";

const mockChatOverview: ChatOverview = {
	cost: { total_cost_micros: 1_250_000, unpriced_request_count: 0 },
	isSubagent: false,
	repos: [
		{
			root: "/home/coder/coder",
			name: "coder",
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

const mockCleanRepo: ChatOverview["repos"][number] = {
	root: "/home/coder/clean",
	name: "clean",
	branch: "main",
	additions: 0,
	deletions: 0,
};

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

/**
 * jsdom has no layout, so the test sets the text widths each row measures,
 * then reports a resize to every row.
 */
const stubTextWidths = () => {
	MockResizeObserver.reset();
	vi.stubGlobal("ResizeObserver", MockResizeObserver);
	return (widths: { scrollWidth: number; clientWidth: number }) => {
		vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(
			widths.scrollWidth,
		);
		vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(
			widths.clientWidth,
		);
		act(() => {
			for (const observer of MockResizeObserver.instances) {
				observer.simulateResize(widths.clientWidth);
			}
		});
	};
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
	it.each<{ name: RegExp; target: OverviewTarget }>([
		{ name: /Cost/, target: { tabId: "summary" } },
		{
			name: /^coder/,
			target: {
				tabId: "git",
				view: { type: "local", repoRoot: "/home/coder/coder" },
			},
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
			const { onOpenTarget } = renderCard();

			await user.click(screen.getByRole("button", { name }));

			expect(onOpenTarget).toHaveBeenCalledExactlyOnceWith(target);
		},
	);

	it("opens nothing from a repository without changes", async () => {
		const user = userEvent.setup();
		const { onOpenTarget } = renderCard({
			...mockChatOverview,
			repos: [mockCleanRepo],
		});

		await user.click(screen.getByText("clean"));

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

	it("shows a truncated row's full label in a tooltip on hover", async () => {
		const user = userEvent.setup();
		const measureText = stubTextWidths();
		renderCard();
		measureText({ scrollWidth: 200, clientWidth: 100 });

		const row = screen.getByRole("button", { name: /#123 Queue jobs/ });
		await user.hover(row);

		await waitFor(() =>
			expect(row).toHaveAccessibleDescription("#123 Queue jobs"),
		);
	});

	it("adds no tooltip to a row whose label fits", () => {
		const measureText = stubTextWidths();
		renderCard();
		measureText({ scrollWidth: 100, clientWidth: 100 });

		const row = screen.getByRole("button", { name: /#123 Queue jobs/ });
		act(() => row.focus());

		expect(row).not.toHaveAccessibleDescription();
	});

	it("skips a repository without changes in the tab order", async () => {
		const user = userEvent.setup();
		renderCard({
			...mockChatOverview,
			cost: undefined,
			repos: [mockCleanRepo],
		});

		await user.tab();

		expect(
			screen.getByRole("button", { name: /#123 Queue jobs/ }),
		).toHaveFocus();
	});

	it("opens a truncated subagent row's tooltip from the keyboard", async () => {
		const measureText = stubTextWidths();
		renderCard({ ...mockChatOverview, cost: undefined, repos: [] });
		measureText({ scrollWidth: 200, clientWidth: 100 });

		const row = screen.getByRole("link", { name: /Find the flake/ });
		act(() => row.focus());

		await waitFor(() =>
			expect(row).toHaveAccessibleDescription("Find the flake"),
		);
	});

	it("shows a failed earlier attachments request with a retry", async () => {
		const user = userEvent.setup();
		const retry = vi.fn();
		render(
			<ChatOverviewCardView
				overview={{ ...mockChatOverview, attachments: [] }}
				earlierAttachmentsRequest={{
					error: new Error("Failed to get chat attachments."),
					isRetrying: false,
					retry,
				}}
				onOpenTarget={vi.fn()}
			/>,
		);

		const section = screen.getByRole("region", { name: "Attachments" });
		await user.click(within(section).getByRole("button", { name: "Retry" }));
		expect(retry).toHaveBeenCalledOnce();
	});
});
