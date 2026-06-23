import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	MockChatContextClean,
	MockChatContextDirty,
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
		// MCP configs are listed by file basename and servers by name.
		expect(body.getByText("MCP")).toBeVisible();
		expect(body.getByText(".mcp.json")).toBeVisible();
		expect(body.getByText("github")).toBeVisible();
		// MCP server tools are listed under their server.
		expect(body.getByText("search_issues")).toBeVisible();
		expect(body.getByText("create_issue")).toBeVisible();
		// Invalid resources are surfaced as issues with their error, not
		// silently dropped.
		expect(body.getByText("Issues")).toBeVisible();
		expect(
			body.getByText(
				'front-matter name "coder-review" does not match directory "moo"',
			),
		).toBeVisible();
		// A clean pin offers no refresh affordance.
		expect(body.queryByRole("button", { name: "Refresh context" })).toBeNull();
	},
};

// Drifted pin: the ring announces a change, and the popover surfaces a refresh
// affordance to re-pin the chat to the latest snapshot.
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
