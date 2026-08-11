import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
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

		// Identifiers wrapped in backticks render as inline code, not literal
		// backticks.
		await expect(canvas.getByText("chatd.go")).toBeInTheDocument();
		await expect(canvas.queryByText(/`/)).not.toBeInTheDocument();

		// Short content fits the bound, so no toggle is offered.
		await expect(
			canvas.queryByRole("button", { name: "Show more" }),
		).not.toBeInTheDocument();
	},
};

// Summaries generated before the structured format are plain prose. They must
// still render, since there is no backfill.
export const LegacyProseSummary: Story = {
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

// A legacy prose summary that happens to start with "1. " parses as an ordered
// list. `ol` is allowlisted so the items keep a list parent instead of
// rendering as orphan `li` elements.
export const LegacyOrderedList: Story = {
	args: { summary: "1. Fixed the race\n2. Added a test" },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const list = canvas.getByRole("list");
		await expect(list.tagName).toBe("OL");
		await expect(within(list).getAllByRole("listitem")).toHaveLength(2);
	},
};

// Links are rendered as plain text. `overflow-hidden` clips the collapsed
// content visually only, so a mounted anchor below the bound would still be
// reachable by keyboard and screen readers while invisible.
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

// The generation prompt preserves identifiers and wraps them in backticks, so
// a single token can be wider than the panel. It has to wrap, because the
// collapsed bound only reveals its toggle for vertical overflow.
export const LongIdentifierWraps: Story = {
	args: {
		summary:
			"Fixed `TestValidateGeneratedChatSummaryHeadlineExceedsBothTheRuneAndSentenceCaps` in `coderd/x/chatd/summarygen_internal_test.go`.",
	},
	// Narrower than the panel's 360px minimum, so the identifier cannot fit on
	// one line.
	decorators: [
		(Story) => (
			<div data-testid="summary-column" className="w-[300px]">
				<Story />
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const column = canvas.getByTestId("summary-column");
		const identifier = canvas.getByText(
			"TestValidateGeneratedChatSummaryHeadlineExceedsBothTheRuneAndSentenceCaps",
		);

		// The identifier has no natural break opportunity, so without a
		// word-breaking rule it renders on one line and escapes the column,
		// where overflow-hidden clips it with no toggle to reveal it.
		await expect(identifier.getBoundingClientRect().right).toBeLessThanOrEqual(
			column.getBoundingClientRect().right + 1,
		);
	},
};

// A cache update can replace the summary in place, without remounting the
// panel, so the overflow toggle has to re-evaluate on new content.
export const SummaryReplacedInPlace: Story = {
	render: (args) => {
		const [summary, setSummary] = useState(MARKDOWN_SUMMARY);
		return (
			<div className="flex flex-col gap-2">
				<button type="button" onClick={() => setSummary(LONG_SUMMARY)}>
					Simulate update
				</button>
				<ChatSummary {...args} summary={summary} />
			</div>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("button", { name: "Show more" }),
		).not.toBeInTheDocument();

		await userEvent.click(
			canvas.getByRole("button", { name: "Simulate update" }),
		);

		await waitFor(async () => {
			await expect(
				canvas.getByRole("button", { name: "Show more" }),
			).toBeInTheDocument();
		});
	},
};

export const LongSummaryExpands: Story = {
	args: { summary: LONG_SUMMARY },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const showMore = await canvas.findByRole("button", { name: "Show more" });
		await userEvent.click(showMore);

		await waitFor(async () => {
			await expect(
				canvas.getByRole("button", { name: "Show less" }),
			).toBeInTheDocument();
		});
	},
};

export const NoSummary: Story = {
	args: { summary: null },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("No summary yet.")).toBeInTheDocument();
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
