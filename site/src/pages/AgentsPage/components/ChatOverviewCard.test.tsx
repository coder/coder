import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MockChatDiffStatus } from "#/testHelpers/chatEntities";
import {
	type ChatOverview,
	ChatOverviewCardView,
	type OverviewTabId,
} from "./ChatOverviewCard";

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
	attachments: [
		{
			key: "1:0",
			role: "assistant",
			file: { type: "file", media_type: "text/html", name: "plan.html" },
			href: "/files/plan.html",
		},
	],
};

const renderCard = () => {
	const onOpenTab = vi.fn<(tabId: OverviewTabId) => void>();
	render(
		<ChatOverviewCardView overview={mockChatOverview} onOpenTab={onOpenTab} />,
	);
	return { onOpenTab };
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
});
