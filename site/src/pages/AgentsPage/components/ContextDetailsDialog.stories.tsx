import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import {
	MockChatContextClean,
	MockChatContextDirty,
} from "#/testHelpers/chatEntities";
import { ContextDetailsDialog } from "./ContextDetailsDialog";

const meta: Meta<typeof ContextDetailsDialog> = {
	title: "pages/AgentsPage/ContextDetailsDialog",
	component: ContextDetailsDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		onRefreshContext: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ContextDetailsDialog>;

const getDialog = async () => {
	const dialog = await within(document.body).findByRole("dialog");
	return within(dialog);
};

// Clean pin: the dialog lists every pinned resource grouped into collapsible
// sections, with no refresh affordance.
export const Clean: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			compressionThreshold: 80,
			context: MockChatContextClean,
		},
	},
	play: async () => {
		const dialog = await getDialog();
		expect(
			dialog.getByText(/6% - 12K \/ 200K context used/),
		).toBeInTheDocument();
		expect(dialog.getByText(/Compacts at 80%/)).toBeInTheDocument();
		expect(dialog.getByText("Context files")).toBeInTheDocument();
		expect(dialog.getByText("AGENTS.md")).toBeInTheDocument();
		expect(dialog.getByText("deploy")).toBeInTheDocument();
		expect(dialog.getByText("MCP")).toBeInTheDocument();
		expect(dialog.getByText("github")).toBeInTheDocument();
		// Invalid resources are surfaced as issues with their error, not
		// silently dropped.
		expect(dialog.getByText("Issues")).toBeInTheDocument();
		expect(
			dialog.getByText(
				'front-matter name "coder-review" does not match directory "moo"',
			),
		).toBeInTheDocument();
		expect(
			dialog.queryByRole("button", { name: "Refresh context" }),
		).toBeNull();
	},
};

// Skill and tool descriptions surface as tooltips to the right of their rows.
export const DescriptionTooltips: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextClean,
		},
	},
	play: async () => {
		const dialog = await getDialog();
		const body = within(document.body);
		await userEvent.hover(dialog.getByText("deploy"));
		// Radix mirrors tooltip content into a visually hidden copy, so
		// assert on presence rather than a unique visible node.
		await waitFor(() => {
			expect(
				body.getAllByText("Deploy the app to staging.").length,
			).toBeGreaterThan(0);
		});
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
	play: async () => {
		const dialog = await getDialog();
		// Both directories that contribute instruction files are listed, so the
		// two AGENTS.md files are no longer ambiguous.
		expect(dialog.getByText("/home/coder")).toBeInTheDocument();
		expect(dialog.getByText("/home/coder/site")).toBeInTheDocument();
		expect(dialog.getAllByText("AGENTS.md")).toHaveLength(2);
		// Skills are grouped under each skill root.
		expect(dialog.getByText("/home/coder/.coder/skills")).toBeInTheDocument();
		expect(dialog.getByText("/home/coder/.agents/skills")).toBeInTheDocument();
		expect(dialog.getByText("deploy")).toBeInTheDocument();
		expect(dialog.getByText("migrate")).toBeInTheDocument();
		expect(dialog.getByText("review")).toBeInTheDocument();
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
	play: async () => {
		const dialog = await getDialog();
		expect(dialog.getByText("/home/coder/.mcp.json")).toBeInTheDocument();
		expect(
			dialog.getByText("/home/coder/project/.mcp.json"),
		).toBeInTheDocument();
	},
};

// Sections, directory groups, and MCP servers all render through the same
// collapsible branch, so one section collapse/expand covers that path.
export const CollapseAndExpand: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextClean,
		},
	},
	play: async () => {
		const dialog = await getDialog();
		const section = dialog.getByRole("button", { name: /Context files/ });
		await userEvent.click(section);
		await waitFor(() => expect(dialog.queryByText("AGENTS.md")).toBeNull());
		await userEvent.click(section);
		await waitFor(() =>
			expect(dialog.getByText("AGENTS.md")).toBeInTheDocument(),
		);
	},
};

// Drifted pin: the dialog mirrors the popover's warning footer and refresh
// affordance.
export const Dirty: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextDirty,
		},
	},
	play: async ({ args }) => {
		const dialog = await getDialog();
		expect(dialog.getByText("Context changed")).toBeInTheDocument();
		expect(
			dialog.getByText(
				"The workspace context changed, so this chat's context files may be outdated.",
			),
		).toBeInTheDocument();

		await userEvent.click(
			dialog.getByRole("button", { name: "Refresh context" }),
		);
		expect(args.onRefreshContext).toHaveBeenCalledTimes(1);
	},
};

// Drift with no resources at all: the dialog still opens (via the warning
// footer) and renders the empty-resources message.
export const DirtyNoResources: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: { dirty: true, resources: [] },
		},
	},
	play: async () => {
		const dialog = await getDialog();
		expect(dialog.getByText("No context resources.")).toBeInTheDocument();
		expect(dialog.getByText("Context changed")).toBeInTheDocument();
	},
};

// Snapshot-level error: the footer surfaces the error message instead of the
// drift warning.
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
	play: async () => {
		const dialog = await getDialog();
		expect(dialog.getByText("Context error")).toBeInTheDocument();
		expect(
			dialog.getByText("failed to read AGENTS.md: permission denied"),
		).toBeInTheDocument();
	},
};
