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

// Keyboard access: the focusable trigger button itself carries the Radix popup
// semantics, and Tab focus (no pointer) opens the popover so keyboard and
// screen-reader users get the same affordance as hover.
export const KeyboardFocusOpensPopover: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextClean,
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		// The popup semantics live on the focusable element, not a wrapper.
		expect(button).toHaveAttribute("aria-haspopup", "dialog");
		expect(button).toHaveAttribute("aria-expanded", "false");

		// Tabbing to the trigger opens the popover and flips aria-expanded.
		await userEvent.tab();
		expect(button).toHaveFocus();
		await waitFor(() =>
			expect(button).toHaveAttribute("aria-expanded", "true"),
		);
		const body = within(document.body);
		await waitFor(() => expect(body.getByText("Context files")).toBeVisible());
	},
};

// Every non-ok resource is surfaced as an issue with its error instead of being
// silently dropped, across kinds (file, MCP config, MCP server, skill) and
// statuses (oversize, unreadable, excluded, invalid).
export const Issues: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: {
				dirty: false,
				resources: [
					{
						source: "/home/coder/big/CLAUDE.md",
						kind: "instruction_file",
						size_bytes: 70_000,
						status: "oversize",
						error: "file exceeds the 64KiB instruction limit",
					},
					{
						source: "/home/coder/secret/AGENTS.md",
						kind: "instruction_file",
						size_bytes: 0,
						status: "unreadable",
						error: "permission denied",
					},
					{
						source: "/home/coder/.coder/skills/legacy",
						kind: "skill",
						size_bytes: 0,
						status: "excluded",
						skill_name: "legacy",
						error: "excluded by .coderignore",
					},
					{
						source: "/home/coder/.mcp.json",
						kind: "mcp_config",
						size_bytes: 0,
						status: "unreadable",
						error: "invalid JSON at line 3",
					},
					{
						source: "broken-server",
						kind: "mcp_server",
						size_bytes: 0,
						status: "invalid",
						error: "failed to start MCP server",
					},
				],
			},
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() => expect(body.getByText("Issues")).toBeVisible());
		// Each non-ok resource shows its name and error.
		expect(body.getByText("CLAUDE.md")).toBeVisible();
		expect(
			body.getByText("file exceeds the 64KiB instruction limit"),
		).toBeVisible();
		expect(body.getByText("permission denied")).toBeVisible();
		expect(body.getByText("legacy")).toBeVisible();
		expect(body.getByText("excluded by .coderignore")).toBeVisible();
		expect(body.getByText("invalid JSON at line 3")).toBeVisible();
		expect(body.getByText("broken-server")).toBeVisible();
		expect(body.getByText("failed to start MCP server")).toBeVisible();
		// The kind label and status accompany the name (leaf span exact match).
		expect(
			body.getAllByText((_, el) => el?.textContent === "(file: oversize)")
				.length,
		).toBeGreaterThan(0);
	},
};

// No usage data: the trigger still announces itself and the popover reports
// that usage is unavailable rather than rendering a broken percentage.
export const UsageUnavailable: Story = {
	args: {
		usage: null,
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		expect(button.getAttribute("aria-label")).toBe("Context usage");
		await userEvent.hover(button);
		const dialog = await within(document.body).findByRole("dialog");
		await waitFor(() =>
			expect(
				within(dialog).getByText("Context usage unavailable"),
			).toBeVisible(),
		);
	},
};

// Empty pinned context: the usage line renders, but with no resources none of
// the resource sections appear.
export const EmptyResources: Story = {
	args: {
		usage: {
			usedTokens: 8_000,
			contextLimitTokens: 200_000,
			context: {
				dirty: false,
				resources: [],
			},
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const dialog = await within(document.body).findByRole("dialog");
		const panel = within(dialog);
		await waitFor(() => expect(panel.getByText(/context used/)).toBeVisible());
		expect(panel.queryByText("Context files")).toBeNull();
		expect(panel.queryByText("Skills")).toBeNull();
		expect(panel.queryByText("MCP")).toBeNull();
		expect(panel.queryByText("Issues")).toBeNull();
	},
};

// Skill and MCP tool descriptions surface as side tooltips on hover.
export const ResourceTooltips: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextClean,
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() => expect(body.getByText("Skills")).toBeVisible());

		// Hovering a skill row reveals its description. Radix renders a
		// visually-hidden copy of the text for assistive tech, so scope the
		// assertion to the visible tooltip role.
		await userEvent.hover(body.getByText("deploy"));
		await waitFor(() =>
			expect(
				body
					.getAllByRole("tooltip")
					.some((tip) =>
						tip.textContent?.includes("Deploy the app to staging."),
					),
			).toBe(true),
		);

		// Hovering an MCP tool row reveals the tool description.
		await userEvent.hover(body.getByText("search_issues"));
		await waitFor(() =>
			expect(
				body
					.getAllByRole("tooltip")
					.some((tip) =>
						tip.textContent?.includes("Search issues and pull requests."),
					),
			).toBe(true),
		);
	},
};

// Duplicate MCP tool names (two tools collide after the "<server>__" prefix is
// stripped) are deduped so a duplicate cannot render twice or produce a
// duplicate React key.
export const DuplicateMcpToolNames: Story = {
	args: {
		usage: {
			usedTokens: 20_000,
			contextLimitTokens: 200_000,
			context: {
				dirty: false,
				resources: [
					{
						source: "github",
						kind: "mcp_server",
						size_bytes: 512,
						status: "ok",
						tools: [
							{ name: "search", description: "First search tool." },
							{ name: "search", description: "Duplicate after prefix strip." },
							{ name: "create", description: "Create something." },
						],
					},
				],
			},
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const body = within(document.body);
		await waitFor(() => expect(body.getByText("MCP")).toBeVisible());
		// The duplicate name renders exactly once.
		expect(body.getAllByText("search")).toHaveLength(1);
		expect(body.getByText("create")).toBeVisible();
	},
};

// Whitespace-only or empty resource names are dropped so a nameless resource
// never renders as a blank row. Only the valid file survives; the
// whitespace-named skill, MCP, and issue entries leave no section behind.
export const DropsEmptyNames: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: {
				dirty: false,
				resources: [
					{
						source: "/home/coder/AGENTS.md",
						kind: "instruction_file",
						size_bytes: 100,
						status: "ok",
					},
					{
						source: "   ",
						kind: "instruction_file",
						size_bytes: 0,
						status: "ok",
					},
					{
						source: "   ",
						kind: "skill",
						size_bytes: 0,
						status: "ok",
						skill_name: "   ",
					},
					{
						source: "   ",
						kind: "mcp_config",
						size_bytes: 0,
						status: "ok",
					},
					{
						source: "   ",
						kind: "mcp_server",
						size_bytes: 0,
						status: "ok",
						tools: [],
					},
					{
						source: "   ",
						kind: "skill",
						size_bytes: 0,
						status: "invalid",
						skill_name: "   ",
						error: "should be dropped",
					},
				],
			},
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.hover(button);
		const dialog = await within(document.body).findByRole("dialog");
		const panel = within(dialog);
		// The single valid file renders.
		await waitFor(() => expect(panel.getByText("AGENTS.md")).toBeVisible());
		// Whitespace-only entries produce neither a section nor a blank row.
		expect(panel.queryByText("Skills")).toBeNull();
		expect(panel.queryByText("MCP")).toBeNull();
		expect(panel.queryByText("Issues")).toBeNull();
		expect(panel.queryByText("should be dropped")).toBeNull();
	},
};
