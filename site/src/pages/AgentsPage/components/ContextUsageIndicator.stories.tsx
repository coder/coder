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
		// A single context root still shows its directory header.
		expect(body.getByText("/home/coder")).toBeVisible();
		expect(body.getByText("/home/coder/.coder/skills")).toBeVisible();
		// The list is driven by the pinned resources.
		expect(body.getByText("AGENTS.md")).toBeVisible();
		expect(body.getByText("deploy")).toBeVisible();
		// MCP configs are listed by full path (so multiple .mcp.json files stay
		// distinct) and servers by name.
		expect(body.getByText("MCP")).toBeVisible();
		expect(body.getByText("/home/coder/.mcp.json")).toBeVisible();
		expect(body.getByText("github")).toBeVisible();
		// MCP server tools are listed under their server.
		expect(body.getByText("search_issues")).toBeVisible();
		expect(body.getByText("create_issue")).toBeVisible();
		// Each populated category shows its total context size.
		expect(body.getByText("(0.2 KiB)")).toBeVisible(); // context files
		expect(body.getByText("(0.1 KiB)")).toBeVisible(); // skills
		expect(body.getByText("(0.7 KiB)")).toBeVisible(); // MCP
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

// Multiple context roots: files and skills are pulled from several
// directories, so each list groups by its parent directory. Without grouping
// the two AGENTS.md files would render as identical, ambiguous rows.
export const MultipleContextRoots: Story = {
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
						skill_description: "Deploy the app to staging.",
					},
					{
						source: "/home/coder/.coder/skills/migrate",
						kind: "skill",
						size_bytes: 120,
						status: "ok",
						skill_name: "migrate",
						skill_description: "Run database migrations.",
					},
					{
						source: "/home/coder/.agents/skills/review",
						kind: "skill",
						size_bytes: 140,
						status: "ok",
						skill_name: "review",
						skill_description: "Review a pull request.",
					},
				],
			},
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		// Both directories that contribute instruction files are listed, so the
		// two AGENTS.md files are no longer ambiguous.
		await waitFor(() => expect(body.getByText("/home/coder")).toBeVisible());
		expect(body.getByText("/home/coder/site")).toBeVisible();
		expect(body.getAllByText("AGENTS.md")).toHaveLength(2);
		// Skills are grouped under each skill root.
		expect(body.getByText("/home/coder/.coder/skills")).toBeVisible();
		expect(body.getByText("/home/coder/.agents/skills")).toBeVisible();
		expect(body.getByText("deploy")).toBeVisible();
		expect(body.getByText("migrate")).toBeVisible();
		expect(body.getByText("review")).toBeVisible();
	},
};

// Multiple .mcp.json files: each config is listed by its full path so the two
// otherwise-identical .mcp.json files stay disambiguated.
export const MultipleMcpConfigs: Story = {
	args: {
		usage: {
			usedTokens: 20_000,
			contextLimitTokens: 200_000,
			context: {
				dirty: false,
				resources: [
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
				],
			},
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() =>
			expect(body.getByText("/home/coder/.mcp.json")).toBeVisible(),
		);
		// The two configs are distinguishable by their full path.
		expect(body.getByText("/home/coder/project/.mcp.json")).toBeVisible();
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

// Before any assistant message reports token usage there is no percentage,
// so the popover explains when the numbers will appear.
export const NoUsage: Story = {
	args: {
		usage: null,
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() =>
			expect(
				body.getByText("Context usage will appear after sending a message."),
			).toBeVisible(),
		);
	},
};

// Some providers report usage without token counts, so no percentage can be
// computed even though a message was sent. The popover must say the usage is
// unavailable instead of promising numbers after the next message.
export const UsageWithoutTokenCounts: Story = {
	args: {
		usage: {
			contextLimitTokens: 200_000,
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() =>
			expect(body.getByText("Context usage unavailable")).toBeVisible(),
		);
	},
};

// A fresh chat has pinned context before any assistant message reports token
// usage, so the popover pairs the empty-usage copy with the resource list.
export const NoUsageWithContext: Story = {
	args: {
		usage: {
			context: MockChatContextClean,
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() =>
			expect(
				body.getByText("Context usage will appear after sending a message."),
			).toBeVisible(),
		);
		expect(body.getByText("Context files")).toBeVisible();
		expect(body.getByText("AGENTS.md")).toBeVisible();
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
