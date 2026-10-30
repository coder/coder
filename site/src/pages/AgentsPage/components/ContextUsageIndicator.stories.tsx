import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import type { ChatContext } from "#/api/typesGenerated";
import {
	MockChatContextACPHarnesses,
	MockChatContextACPHarnessIssue,
	MockChatContextClean,
	MockChatContextDirty,
	MockChatContextResourceFailures,
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

export const CompactTrigger: Story = {
	args: {
		usage: {
			usedTokens: 680_000,
			contextLimitTokens: 1_000_000,
			compactionThreshold: {
				percent: 70,
				source: "user",
				organizationOverrideNotLoaded: false,
			},
		},
	},
};

export const LowUsageTrigger: Story = {
	...CompactTrigger,
	args: {
		usage: {
			...CompactTrigger.args?.usage,
			usedTokens: 10_600,
		},
	},
};

export const LowUsage: Story = {
	...LowUsageTrigger,
	play: async ({ canvasElement }) => {
		await userEvent.click(within(canvasElement).getByRole("button"));
	},
};

export const OrganizationCompactionPointUsesReportedLimit: Story = {
	args: {
		usage: {
			usedTokens: 64_000,
			contextLimitTokens: 200_000,
			compactionThreshold: {
				source: "organization",
				pointTokens: 32_000,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /Context usage 32%/i }),
		);
	},
};

export const OrganizationCompactionPointBeyondReportedLimit: Story = {
	args: {
		usage: {
			usedTokens: 5_200,
			contextLimitTokens: 10_000,
			compactionThreshold: {
				source: "organization",
				pointTokens: 16_000,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /Context usage 52%/i }),
		);
	},
};

export const ChatModelCompactionDisabled: Story = {
	args: {
		usage: {
			usedTokens: 64_000,
			contextLimitTokens: 128_000,
			compactionThreshold: {
				percent: 100,
				source: "user",
				organizationOverrideNotLoaded: false,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /Context usage 50%/i }),
		);
	},
};

export const ChatModelCompactionDisabledOrganizationOverrideNotLoaded: Story = {
	args: {
		usage: {
			usedTokens: 64_000,
			contextLimitTokens: 128_000,
			compactionThreshold: {
				percent: 100,
				source: "user",
				organizationOverrideNotLoaded: true,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /Context usage 50%/i }),
		);
	},
};

export const ChatModelCompactionBinding: Story = {
	args: {
		usage: {
			usedTokens: 64_000,
			contextLimitTokens: 128_000,
			compactionThreshold: {
				percent: 80,
				source: "model",
				organizationOverrideNotLoaded: false,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: /Context usage 50%/i }),
		);
	},
};

export const HighUsage: Story = {
	args: {
		usage: { usedTokens: 90_000, contextLimitTokens: 100_000 },
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(within(canvasElement).getByRole("button"));
	},
};

export const CriticalUsage: Story = {
	...HighUsage,
	args: {
		usage: { usedTokens: 95_000, contextLimitTokens: 100_000 },
	},
};

export const OverCapacity: Story = {
	...HighUsage,
	args: {
		usage: { usedTokens: 120_000, contextLimitTokens: 100_000 },
	},
};

export const UserCompactionThreshold: Story = {
	...HighUsage,
	args: {
		usage: {
			usedTokens: 680_000,
			contextLimitTokens: 1_000_000,
			compactionThreshold: {
				percent: 70,
				source: "user",
				organizationOverrideNotLoaded: false,
			},
		},
	},
};

export const BeforeCompactionWarning: Story = {
	...HighUsage,
	args: {
		usage: {
			...UserCompactionThreshold.args?.usage,
			usedTokens: 629_000,
		},
	},
};

export const AtCompactionWarning: Story = {
	...HighUsage,
	args: {
		usage: {
			...UserCompactionThreshold.args?.usage,
			usedTokens: 630_000,
		},
	},
};

export const BeforeWindowWarning: Story = {
	...HighUsage,
	args: {
		usage: { usedTokens: 89_000, contextLimitTokens: 100_000 },
	},
};

export const EstimatedUsage: Story = {
	...HighUsage,
	args: {
		usage: {
			...ChatModelCompactionBinding.args?.usage,
			estimated: true,
		},
	},
};

export const ZeroUsage: Story = {
	...HighUsage,
	args: {
		usage: { usedTokens: 0, contextLimitTokens: 100_000 },
	},
};

export const OrganizationCompactionAtWindowLimit: Story = {
	...HighUsage,
	args: {
		usage: {
			usedTokens: 50_000,
			contextLimitTokens: 100_000,
			compactionThreshold: {
				source: "organization",
				pointTokens: 100_000,
			},
		},
	},
};

const mockChatContextACPHarnessesWithOtherContext: ChatContext = {
	...MockChatContextClean,
	resources: [
		...(MockChatContextClean.resources ?? []),
		...(MockChatContextACPHarnesses.resources ?? []),
	],
};

const mockChatContextACPHarnessesWithoutSourcePath: ChatContext = {
	...MockChatContextACPHarnesses,
	resources: MockChatContextACPHarnesses.resources?.map((resource) => ({
		...resource,
		source_path: undefined,
	})),
};

export const ACPHarnesses: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextACPHarnesses,
		},
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(within(canvasElement).getByRole("button"));
		await userEvent.click(screen.getByRole("button", { name: /^ACP/ }));
	},
};

export const ACPHarnessesWithOtherContext: Story = {
	...ACPHarnesses,
	args: {
		usage: {
			...ACPHarnesses.args?.usage,
			context: mockChatContextACPHarnessesWithOtherContext,
		},
	},
};

export const ACPHarnessIssue: Story = {
	...ACPHarnesses,
	args: {
		usage: {
			...ACPHarnesses.args?.usage,
			context: MockChatContextACPHarnessIssue,
		},
	},
};

export const ACPHarnessesWithoutSourcePath: Story = {
	...ACPHarnesses,
	args: {
		usage: {
			...ACPHarnesses.args?.usage,
			context: mockChatContextACPHarnessesWithoutSourcePath,
		},
	},
};

// A failed skill appears inside its section alongside available skills.
export const ResourceIssue: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextClean,
		},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.click(button);
		await userEvent.click(screen.getByRole("button", { name: /Skills/ }));
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
		await userEvent.click(button);
		await userEvent.click(
			screen.getByRole("button", { name: /Context files/ }),
		);
		await userEvent.click(screen.getByRole("button", { name: /Skills/ }));
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
		await userEvent.click(button);
		await userEvent.click(screen.getByRole("button", { name: /MCP/ }));
	},
};

// Drifted pin: the indicator announces a change, and the popover surfaces a refresh
// affordance to re-pin the chat to the latest snapshot.
export const Dirty: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextDirty,
		},
	},
	play: async ({ canvasElement }) => {
		await userEvent.click(within(canvasElement).getByRole("button"));
	},
};

// Before any assistant message reports token usage there is no percentage,
// so the popover explains when the numbers will appear.
export const NoUsage: Story = {
	args: {
		usage: {},
	},
	play: async ({ canvasElement }) => {
		const button = within(canvasElement).getByRole("button");
		await userEvent.click(button);
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
		await userEvent.click(button);
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
		await userEvent.click(button);
	},
};

// Snapshot-level error: the indicator shows a distinct error treatment and the
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
		await userEvent.click(button);
	},
};

export const ExpandedResources: Story = {
	...NoUsageWithContext,
	play: async ({ canvasElement }) => {
		await userEvent.click(within(canvasElement).getByRole("button"));
		const dialog = screen.getByRole("dialog");
		for (const name of [/Context files/, /Skills/, /MCP/]) {
			await userEvent.click(within(dialog).getByRole("button", { name }));
		}
	},
};

export const ResourceFailuresOnly: Story = {
	args: { usage: { context: MockChatContextResourceFailures } },
	play: async ({ canvasElement }) => {
		await userEvent.click(within(canvasElement).getByRole("button"));
	},
};

export const ResourceFailuresOnlyExpanded: Story = {
	...ResourceFailuresOnly,
	play: async ({ canvasElement }) => {
		await userEvent.click(within(canvasElement).getByRole("button"));
		for (const name of [/Context files/, /Skills/, /MCP Servers/, /^ACP/]) {
			await userEvent.click(screen.getByRole("button", { name }));
		}
	},
};
