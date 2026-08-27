import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { ChatSummary } from "./ChatSummary";

const MARKDOWN_SUMMARY = [
	"Investigated the flaky CI job in `coderd/x/chatd` and landed a fix.",
	"",
	"- Traced the failure to a cache-layer race in `chatd.go`",
	"- Added a regression test covering the race",
	"- Opened PR #26649",
].join("\n");

const LONG_SUMMARY = [
	"Audited the whole chat pipeline and shipped a batch of fixes.",
	"",
	...Array.from(
		{ length: 12 },
		(_, i) =>
			`- Reviewed subsystem number ${i + 1} and applied the corresponding fix so the behaviour matches the specification`,
	),
].join("\n");

const meta: Meta<typeof ChatSummary> = {
	title: "pages/AgentsPage/ChatSummary",
	component: ChatSummary,
	args: {
		summary: MARKDOWN_SUMMARY,
		createdAt: "2024-05-01T12:00:00Z",
		updatedAt: "2024-05-02T15:30:00Z",
		costMicros: 1_250_000,
		showCost: true,
	},
	decorators: [
		(Story) => (
			<div className="w-[400px] max-w-full">
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
		await expect(canvas.getByText("Created:")).toBeInTheDocument();
		await expect(canvas.getByText("Updated:")).toBeInTheDocument();
		await expect(canvas.getByText("Cost:")).toBeInTheDocument();
		await expect(canvas.getByText("May 1, 2024")).toBeInTheDocument();
		await expect(canvas.getByText("May 2, 2024")).toBeInTheDocument();
		await expect(canvas.queryByText(/12:00|15:30/)).not.toBeInTheDocument();
		// formatCostMicros is locale-pinned to en-US, so this is deterministic.
		await expect(canvas.getByText("$1.25")).toBeInTheDocument();
	},
};

export const HeadlineAndBullets: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Investigated the flaky CI job/),
		).toBeInTheDocument();

		const list = canvas.getByRole("list");
		await expect(within(list).getAllByRole("listitem")).toHaveLength(3);

		// Backticked identifiers render as inline code, not literal backticks.
		await expect(canvas.getByText("chatd.go")).toBeInTheDocument();
		await expect(canvas.queryByText(/`/)).not.toBeInTheDocument();
	},
};

// Headline-only summaries: legacy prose, subagent report snippets, and
// trivial chats whose headline covers everything.
export const HeadlineOnlySummary: Story = {
	args: {
		summary:
			"Investigated the flaky CI job, traced it to a race in the cache layer, and added a regression test.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/traced it to a race in the cache layer/),
		).toBeInTheDocument();
		await expect(canvas.queryByRole("list")).not.toBeInTheDocument();
	},
};

// A prose summary starting with "1. " parses as an ordered list; `ol` is
// allowlisted so the items keep a list parent.
export const LegacyOrderedList: Story = {
	args: { summary: "1. Fixed the race\n2. Added a test" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const list = canvas.getByRole("list");
		await expect(list.tagName).toBe("OL");
		await expect(within(list).getAllByRole("listitem")).toHaveLength(2);
	},
};

export const LinksRenderAsPlainText: Story = {
	args: {
		summary:
			"Investigated the failure in [PR #26649](https://example.com/pr) and fixed it.",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(/PR #26649/)).toBeInTheDocument();
		await expect(canvas.queryByRole("link")).not.toBeInTheDocument();
	},
};

// A single backticked identifier can be wider than the panel and must wrap.
export const LongIdentifierWraps: Story = {
	args: {
		summary:
			"Fixed `TestValidateGeneratedChatSummaryHeadlineExceedsBothTheRuneAndSentenceCaps` in `coderd/x/chatd/summarygen_internal_test.go`.",
	},
	// Narrower than the panel minimum so the identifier cannot fit on one
	// line. Layout is covered by visual snapshots; per FE10 the assertion
	// stays semantic.
	decorators: [
		(Story) => (
			<div className="w-[300px]">
				<Story />
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(
				"TestValidateGeneratedChatSummaryHeadlineExceedsBothTheRuneAndSentenceCaps",
			),
		).toBeVisible();
	},
};

// Renders at natural height; the surrounding panel scrolls instead of
// clipping or collapsing.
export const LongSummary: Story = {
	args: { summary: LONG_SUMMARY },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const list = canvas.getByRole("list");
		await expect(within(list).getAllByRole("listitem")).toHaveLength(12);
		await expect(within(list).getByText(/subsystem number 12/)).toBeVisible();
	},
};

export const NoSummary: Story = {
	args: { summary: null },
	decorators: [
		(Story) => (
			<div className="flex h-[420px] w-[400px] max-w-full flex-col border border-solid border-border-default">
				<Story />
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Not enough details to summarize."),
		).toBeInTheDocument();
		await expect(
			canvas.getByText(
				"A recap of your chat will appear here after a few more messages.",
			),
		).toBeInTheDocument();
		await expect(
			canvas.queryByText("Generating summary"),
		).not.toBeInTheDocument();
		await expect(canvas.getByText("Created:")).toBeInTheDocument();
		await expect(canvas.getByText("Updated:")).toBeInTheDocument();
		await expect(canvas.getByText("Cost:")).toBeInTheDocument();
		await expect(canvas.getByText("$1.25")).toBeInTheDocument();
	},
};

export const GeneratingSummary: Story = {
	args: { summary: null, isGenerating: true },
	decorators: [
		(Story) => (
			<div className="flex h-[420px] w-[400px] max-w-full flex-col border border-solid border-border-default">
				<Story />
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const status = canvas.getByRole("status");
		await expect(status).toHaveTextContent("Generating summary");
		await expect(
			canvas.queryByText("Not enough details to summarize."),
		).not.toBeInTheDocument();
		await expect(canvas.getByText("Created:")).toBeInTheDocument();
		await expect(canvas.getByText("Cost:")).toBeInTheDocument();
	},
};

// A subagent's summary is its final report, persisted when it
// completes, so an empty summary means the agent is still working.
export const SubagentSummaryPending: Story = {
	args: { summary: null, isSubagent: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Summary pending agent completion."),
		).toBeInTheDocument();
	},
};

export const CostLoading: Story = {
	args: { isCostLoading: true, costMicros: undefined },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText("Loading cost")).toBeInTheDocument();
	},
};

export const CostAbsent: Story = {
	args: { costMicros: null },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Cost:")).toBeInTheDocument();
		await expect(canvas.getByText("-")).toBeInTheDocument();
	},
};

export const SubCentCost: Story = {
	args: { costMicros: 5_000 },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("$0.0050")).toBeInTheDocument();
	},
};

export const CostError: Story = {
	args: { costMicros: undefined, costError: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Cost:")).toBeInTheDocument();
		await expect(canvas.getByText("Unavailable")).toBeInTheDocument();
	},
};

export const PartialCost: Story = {
	args: { costMicros: 0, unpricedRequestCount: 3 },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Excludes unpriced usage from 3 requests."),
		).toBeInTheDocument();
	},
};

export const SubagentTreeCost: Story = {
	args: { isSubagent: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Cost covers this agent's whole chat/),
		).toBeInTheDocument();
	},
};

export const CostHidden: Story = {
	args: { showCost: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Updated:")).toBeInTheDocument();
		await expect(canvas.queryByText("Cost:")).not.toBeInTheDocument();
		await expect(canvas.queryByText("$1.25")).not.toBeInTheDocument();
	},
};
