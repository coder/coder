import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent } from "storybook/test";
import { MockAIBridgeSessionNetworkCalls } from "#/testHelpers/entities";
import { NetworkCallsTable } from "./NetworkCallsTable";

const meta: Meta<typeof NetworkCallsTable> = {
	title: "pages/AIBridgePage/NetworkCallsTable",
	component: NetworkCallsTable,
	args: {
		summary: { total: 4, blocked: 2 },
		calls: MockAIBridgeSessionNetworkCalls,
	},
};

export default meta;
type Story = StoryObj<typeof NetworkCallsTable>;

export const Default: Story = {
	play: async ({ canvas }) => {
		await canvas.findByText("Network calls (4)");
		await expect(canvas.getAllByText("Allowed")).toHaveLength(2);
		await expect(canvas.getAllByText("Blocked")).toHaveLength(2);
		await expect(
			canvas.getByText("https://registry.npmjs.org/lodash"),
		).toBeInTheDocument();
	},
};

// The header badge counts every blocked call in the session, so it can exceed
// the number of blocked rows on screen once the list is capped.
export const BlockedBadge: Story = {
	args: {
		summary: { total: 4, blocked: 9 },
	},
	play: async ({ canvas }) => {
		await expect(
			canvas.getByText((_content, element) => {
				return element?.textContent === "Blocked network calls: 9";
			}),
		).toBeInTheDocument();
		await expect(canvas.getAllByText("Blocked")).toHaveLength(2);
	},
};

export const NoBlockedCalls: Story = {
	args: {
		summary: { total: 1, blocked: 0 },
		calls: [MockAIBridgeSessionNetworkCalls[0]],
	},
	play: async ({ canvas }) => {
		await canvas.findByText("Network calls (1)");
		await expect(canvas.queryByText("Blocked")).not.toBeInTheDocument();
	},
};

export const ExpandRow: Story = {
	play: async ({ canvas }) => {
		await expect(canvas.queryByText("Protocol")).not.toBeInTheDocument();

		await userEvent.click(
			canvas.getByRole("button", {
				name: /https:\/\/api\.github\.com\/repos\/coder\/coder/,
			}),
		);

		await expect(canvas.getByText("Protocol")).toBeInTheDocument();
		await expect(canvas.getByText("Matched rule")).toBeInTheDocument();
	},
};

export const CollapsePanel: Story = {
	play: async ({ canvas }) => {
		await expect(
			canvas.getByText("https://api.github.com/repos/coder/coder"),
		).toBeVisible();

		await userEvent.click(canvas.getByText("Network calls (4)"));

		await expect(
			canvas.queryByText("https://api.github.com/repos/coder/coder"),
		).not.toBeInTheDocument();
	},
};

export const Empty: Story = {
	args: {
		summary: { total: 0, blocked: 0 },
		calls: [],
	},
	play: async ({ canvas }) => {
		await canvas.findByText("Network calls (0)");
		await expect(
			canvas.getByText("No network calls were recorded for this session."),
		).toBeInTheDocument();
	},
};

// A summary total with no rows to show is not a state the server produces. The
// panel still shows a single message rather than claiming both that nothing was
// recorded and that the list was capped.
export const EmptyListWithSummaryTotal: Story = {
	args: {
		summary: { total: 4, blocked: 2 },
		calls: [],
	},
	play: async ({ canvas }) => {
		await expect(
			canvas.getByText("No network calls were recorded for this session."),
		).toBeInTheDocument();
		await expect(
			canvas.queryByText(/Showing the first/),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByText(/matches within the first/),
		).not.toBeInTheDocument();
	},
};

// When the session has more calls than the server returns, the panel notes
// how many are shown.
export const Truncated: Story = {
	args: {
		summary: { total: 150, blocked: 2 },
		calls: MockAIBridgeSessionNetworkCalls,
	},
	play: async ({ canvas }) => {
		await canvas.findByText("Network calls (150)");
		await expect(
			canvas.getByText(/Showing the first 4 of 150 network calls\./),
		).toBeInTheDocument();
	},
};

// While searching, the header reports the match count and the note surfaces
// that the search only covered the loaded (truncated) prefix.
export const SearchTruncated: Story = {
	args: {
		summary: { total: 150, blocked: 2 },
		calls: [MockAIBridgeSessionNetworkCalls[0]],
		search: { loaded: 4 },
	},
	play: async ({ canvas }) => {
		await canvas.findByText("1 match");
		await expect(
			canvas.getByText(/1 match within the first 4 of 150 network calls\./),
		).toBeInTheDocument();
	},
};

// Zero matches over a truncated session still renders the panel so the
// truncation caveat is disclosed even when the search found nothing.
export const SearchTruncatedNoMatches: Story = {
	args: {
		summary: { total: 150, blocked: 2 },
		calls: [],
		search: { loaded: 4 },
	},
	play: async ({ canvas }) => {
		await expect(
			canvas.getByText("No network calls match your search."),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(/0 matches within the first 4 of 150 network calls\./),
		).toBeInTheDocument();
	},
};
