import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { MockResizeObserver } from "#/testHelpers/resizeObserver";
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
	renderComponent(
		<ChatOverviewCardView overview={overview} onOpenTarget={onOpenTarget} />,
	);
	return { onOpenTarget };
};

describe("ChatOverviewCardView", () => {
	it.each<{ name: RegExp; tab: OverviewTarget["tabId"] }>([
		{ name: /Cost/, tab: "summary" },
		{ name: /^coder/, tab: "git" },
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
			repos: [mockCleanRepo],
		});

		await user.click(screen.getByText("clean"));

		expect(onOpenTarget).not.toHaveBeenCalled();
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
});
