import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
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
		// Header shows the total from the summary.
		await canvas.findByText("Network calls (4)");
		// Both allowed and blocked calls render with their status labels.
		await expect(canvas.getAllByText("Allowed")).toHaveLength(2);
		await expect(canvas.getAllByText("Blocked")).toHaveLength(2);
		// A blocked destination from the mock is listed.
		await expect(
			canvas.getByText("https://registry.npmjs.org/lodash"),
		).toBeInTheDocument();
	},
};

// The blocked-count badge in the header reflects the summary, not the number
// of rendered rows.
export const BlockedBadge: Story = {
	play: async ({ canvas }) => {
		const header = canvas.getByText("Network calls (4)").closest("div");
		if (!header) {
			throw new Error("network calls header not found");
		}
		await expect(within(header).getByText("2")).toBeInTheDocument();
	},
};

export const NoBlockedCalls: Story = {
	args: {
		summary: { total: 1, blocked: 0 },
		calls: [MockAIBridgeSessionNetworkCalls[0]],
	},
	play: async ({ canvas }) => {
		await canvas.findByText("Network calls (1)");
		// With zero blocked calls the warning badge is omitted.
		await expect(canvas.queryByText("Blocked")).not.toBeInTheDocument();
	},
};

// Expanding a row reveals its detail fields.
export const ExpandRow: Story = {
	play: async ({ canvas }) => {
		await expect(canvas.queryByText("Protocol")).not.toBeInTheDocument();

		const rowButtons = canvas.getAllByRole("button", { expanded: false });
		// The first row button toggles the first call's detail open.
		await userEvent.click(rowButtons[0]);

		await expect(canvas.getByText("Protocol")).toBeInTheDocument();
		await expect(canvas.getByText("Matched rule")).toBeInTheDocument();
	},
};

// Collapsing the panel header hides the list of calls.
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
