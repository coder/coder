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

// Clean pin: the ring carries no change marker and hovering shows the compact
// summary with counts and the full-list affordance. The full listing itself
// lives in the details dialog (see ContextDetailsDialog stories).
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
		await waitFor(() =>
			expect(body.getByText("6% - 12K / 200K context used")).toBeVisible(),
		);
		expect(body.getByText("1 context file · 1 skill")).toBeVisible();
		expect(body.getByText("1 MCP config · 1 server · 2 tools")).toBeVisible();
		expect(body.getByText("1 issue")).toBeVisible();
		expect(body.getByText("Click to open full list")).toBeVisible();
		// The full listing is not rendered inline anymore.
		expect(body.queryByText("AGENTS.md")).toBeNull();
		expect(body.queryByRole("button", { name: "Refresh context" })).toBeNull();
	},
};

// Plural counts: zero-count segments are omitted and the "MCP" prefix
// attaches to the first MCP segment present.
export const ManyResources: Story = {
	args: {
		usage: {
			usedTokens: 48_000,
			contextLimitTokens: 200_000,
			context: {
				dirty: false,
				resources: [
					{
						source: "/home/coder/AGENTS.md",
						kind: "instruction_file",
						size_bytes: 248,
						status: "ok",
					},
					{
						source: "/home/coder/site/AGENTS.md",
						kind: "instruction_file",
						size_bytes: 512,
						status: "ok",
					},
					{
						source: "/home/coder/.coder/skills/deploy",
						kind: "skill",
						size_bytes: 96,
						status: "ok",
						skill_name: "deploy",
					},
					{
						source: "/home/coder/.coder/skills/migrate",
						kind: "skill",
						size_bytes: 120,
						status: "ok",
						skill_name: "migrate",
					},
					{
						source: "/home/coder/.agents/skills/review",
						kind: "skill",
						size_bytes: 140,
						status: "ok",
						skill_name: "review",
					},
					{
						source: "/home/coder/.mcp.json",
						kind: "mcp_config",
						size_bytes: 184,
						status: "ok",
					},
					{
						source: "/home/coder/project/.mcp.json",
						kind: "mcp_config",
						size_bytes: 256,
						status: "ok",
					},
					{
						source: "github",
						kind: "mcp_server",
						size_bytes: 512,
						status: "ok",
						tools: [{ name: "search_issues" }, { name: "create_issue" }],
					},
				],
			},
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() =>
			expect(body.getByText("2 context files · 3 skills")).toBeVisible(),
		);
		expect(body.getByText("2 MCP configs · 1 server · 2 tools")).toBeVisible();
		expect(body.queryByText("1 issue")).toBeNull();
	},
};

// The details dialog opens from the ring itself and from the popover's link
// row, and the popover never lingers underneath it.
export const OpensDetailsDialog: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextClean,
		},
	},
	play: async ({ canvasElement, step }) => {
		const button = within(canvasElement).getByRole("button");
		const body = within(document.body);
		expect(button.getAttribute("aria-label") ?? "").toContain(
			"Click to open context details.",
		);

		await step("clicking the ring opens the details dialog", async () => {
			await userEvent.click(button);
			const dialog = await body.findByRole("dialog");
			expect(within(dialog).getByText("Context details")).toBeInTheDocument();
			// The compact popover has closed.
			await waitFor(() =>
				expect(body.queryByText("Click to open full list")).toBeNull(),
			);
		});

		await step("escape closes the dialog", async () => {
			await userEvent.keyboard("{Escape}");
			await waitFor(() => expect(body.queryByRole("dialog")).toBeNull());
		});

		await step("the popover link row also opens the dialog", async () => {
			await userEvent.hover(button);
			const link = await body.findByRole("button", {
				name: "Click to open full list",
			});
			await userEvent.click(link);
			const dialog = await body.findByRole("dialog");
			expect(within(dialog).getByText("Context details")).toBeInTheDocument();
		});
	},
};

// Drifted pin: the ring announces a change, and the popover surfaces a
// refresh affordance to re-pin the chat to the latest snapshot.
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

// No pinned context at all: the popover shows only the usage line, the link
// row is hidden, and clicking the ring is a no-op.
export const Empty: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		expect(button.getAttribute("aria-label") ?? "").not.toContain(
			"Click to open context details.",
		);

		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() =>
			expect(body.getByText("6% - 12K / 200K context used")).toBeVisible(),
		);
		expect(body.queryByText("Click to open full list")).toBeNull();

		// Without details, clicking the ring does not open the dialog. The
		// popover content itself carries role="dialog", so assert on the
		// details dialog title instead of the role.
		await userEvent.click(button);
		expect(body.queryByText("Context details")).toBeNull();
	},
};
