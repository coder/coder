import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { ChatSummary } from "./ChatSummary";

const meta: Meta<typeof ChatSummary> = {
	title: "pages/AgentsPage/ChatSummary",
	component: ChatSummary,
	args: {
		summary: [
			"Defines how chat summaries are generated and rendered.",
			"",
			"- Replaces the prompt in `coderd/x/chatd/quickgen.go:916`",
			"- Traces the flaky job to a race in `cache.go:212`",
			"- Adds a regression test in `cache_test.go`",
		].join("\n"),
		createdAt: "2024-05-01T12:00:00Z",
		updatedAt: "2024-05-02T15:30:00Z",
		costMicros: 1_250_000,
		showCost: true,
	},
	decorators: [
		(Story) => (
			<div className="w-[400px] max-w-full p-4">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ChatSummary>;

export const WithSummary: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Defines how chat summaries are generated/),
		).toBeInTheDocument();

		const list = canvas.getByRole("list");
		await expect(within(list).getAllByRole("listitem")).toHaveLength(3);

		// Backticked identifiers render as inline code, not literal backticks.
		await expect(canvas.getByText("cache.go:212").tagName).toBe("CODE");
		await expect(canvas.queryByText(/`/)).not.toBeInTheDocument();
	},
};

// A headline alone is valid when it already covers the whole chat, and
// subagent summaries are plain report prose.
export const ProseSummary: Story = {
	args: {
		summary: "Fixes a typo in `README.md`.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/Fixes a typo in/)).toBeInTheDocument();
		await expect(canvas.queryByRole("list")).not.toBeInTheDocument();
	},
};

// A legacy prose summary starting with "1. " parses as an ordered list; `ol`
// is allowlisted so the items keep a list parent.
export const LegacyOrderedList: Story = {
	args: { summary: "1. Fixed the race\n2. Added a test" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const list = canvas.getByRole("list");
		await expect(list.tagName).toBe("OL");
		await expect(within(list).getAllByRole("listitem")).toHaveLength(2);
	},
};

// A summary describes the chat rather than linking out of it, so a
// model-authored URL keeps its text and drops the anchor.
export const LinksRenderAsPlainText: Story = {
	args: {
		summary: "Changes the summary prompt in [PR #29203](https://example.com).",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/PR #29203/)).toBeInTheDocument();
		await expect(canvas.queryByRole("link")).not.toBeInTheDocument();
	},
};

export const NoSummary: Story = {
	args: { summary: null },
};

// A subagent's summary is its final report, persisted when it
// completes, so an empty summary means the agent is still working.
export const SubagentSummaryPending: Story = {
	args: { summary: null, isSubagent: true },
};

export const CostLoading: Story = {
	args: { isCostLoading: true, costMicros: undefined },
};

export const CostAbsent: Story = {
	args: { costMicros: null },
};

export const SubCentCost: Story = {
	args: { costMicros: 5_000 },
};

export const CostError: Story = {
	args: { costMicros: undefined, costError: true },
};

export const PartialCost: Story = {
	args: { costMicros: 0, unpricedRequestCount: 3 },
};

export const SubagentTreeCost: Story = {
	args: { isSubagent: true },
};

export const CostHidden: Story = {
	args: { showCost: false },
};
