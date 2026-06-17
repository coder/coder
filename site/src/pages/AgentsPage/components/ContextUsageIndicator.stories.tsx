import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	MockChatContextClean,
	MockChatContextDirty,
	MockLastInjectedContextEmptyFile,
} from "#/testHelpers/chatEntities";
import { ContextUsageIndicator } from "./ContextUsageIndicator";

const meta: Meta<typeof ContextUsageIndicator> = {
	title: "pages/AgentsPage/ContextUsageIndicator",
	component: ContextUsageIndicator,
	args: {
		onRefreshContext: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ContextUsageIndicator>;

// Clean pin: the ring carries no change marker and the popover lists the
// pinned resources.
export const Clean: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextClean,
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		expect(button.getAttribute("aria-label") ?? "").not.toContain(
			"Context changed",
		);

		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() => expect(body.getByText("Context files")).toBeVisible());
		// The list is driven by the pinned resources.
		expect(body.getByText("AGENTS.md")).toBeVisible();
		expect(body.getByText("deploy")).toBeVisible();
		// A clean pin offers no refresh affordance.
		expect(body.queryByRole("button", { name: "Refresh context" })).toBeNull();
	},
};

// Drifted pin: the ring announces a change, and the popover surfaces refresh
// and a way into the changes dialog.
export const Dirty: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextDirty,
		},
	},
	play: async ({ canvasElement, args }) => {
		const button = within(canvasElement).getByRole("button");
		expect(button.getAttribute("aria-label") ?? "").toContain(
			"Context changed",
		);

		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() =>
			expect(body.getByText("Context changed")).toBeVisible(),
		);

		// Refresh from the popover invokes the handler.
		await userEvent.click(
			body.getByRole("button", { name: "Refresh context" }),
		);
		expect(args.onRefreshContext).toHaveBeenCalledTimes(1);

		// "View changes" opens the diff dialog.
		await userEvent.click(body.getByRole("button", { name: "View changes" }));
		await waitFor(() =>
			expect(body.getByText("Context changes")).toBeVisible(),
		);
		// The modified skill is listed by name in the dialog.
		expect(body.getByText("Deploy the app to production.")).toBeVisible();
	},
};

// Regression: a dirty pin whose pinned resources have not loaded falls back to
// the agent's injected context, which can carry an empty context-file marker.
// The popover must skip it rather than render a nameless "Context files" row,
// while still surfacing the drift affordances.
export const DirtyEmptyInjectedContext: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			lastInjectedContext: MockLastInjectedContextEmptyFile,
			context: {
				dirty: true,
				changes: MockChatContextDirty.changes,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		// The drift affordances still render.
		await waitFor(() =>
			expect(body.getByText("Context changed")).toBeVisible(),
		);
		expect(body.getByRole("button", { name: "Refresh context" })).toBeVisible();
		// The empty injected marker must not produce a nameless file list.
		expect(body.queryByText("Context files")).toBeNull();
	},
};

// Snapshot-level error: the ring shows a distinct error treatment and the
// popover surfaces the error message.
export const SnapshotError: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: {
				dirty: false,
				error: "failed to read AGENTS.md: permission denied",
				resources: MockChatContextClean.resources,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() => expect(body.getByText("Context error")).toBeVisible());
		expect(
			body.getByText("failed to read AGENTS.md: permission denied"),
		).toBeVisible();
	},
};
