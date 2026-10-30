import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fireEvent, fn, screen, userEvent, within } from "storybook/test";
import type { ChatContext, ChatContextResource } from "#/api/typesGenerated";
import { Button } from "#/components/Button/Button";
import {
	MockChatContextACPHarnesses,
	MockChatContextACPHarnessIssue,
	MockChatContextClean,
	MockChatContextDirty,
	MockChatContextResourceFailures,
} from "#/testHelpers/chatEntities";
import {
	type AgentContextUsage,
	ContextUsageIndicator,
} from "./ContextUsageIndicator";

// Only the OK rows of the shared fixture, so the discovery stories show one
// MCP state at a time without the fixture's invalid skill under Issues.
const MockChatContextResourcesHealthy: readonly ChatContextResource[] =
	MockChatContextClean.resources?.filter(
		(resource) => resource.status === "ok",
	) ?? [];

// Pinned context without any MCP rows, so the MCP section is driven by the
// discovery state alone.
const MockChatContextResourcesWithoutMcp: readonly ChatContextResource[] =
	MockChatContextResourcesHealthy.filter(
		(resource) =>
			resource.kind !== "mcp_config" && resource.kind !== "mcp_server",
	);

const MockChatContextMcpPending: ChatContext = {
	...MockChatContextClean,
	resources: MockChatContextResourcesWithoutMcp,
	mcp_discovery: { phase: "pending", stale: false },
};

const MockChatContextMcpCompleteNoServers: ChatContext = {
	...MockChatContextClean,
	resources: MockChatContextResourcesWithoutMcp,
	mcp_discovery: { phase: "complete", stale: false },
};

const MockChatContextMcpComplete: ChatContext = {
	...MockChatContextClean,
	resources: MockChatContextResourcesHealthy,
	mcp_discovery: { phase: "complete", stale: false },
};

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

// Opens the popover and expands its MCP section, where the discovery
// state is shown.
const openMcpSection = async (canvasElement: HTMLElement) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: /Context usage/ }),
	);
	await userEvent.click(screen.getByRole("button", { name: /MCP Servers/ }));
};

// The agent has not finished its initial MCP reload: the MCP section explains
// that discovery is still initializing instead of showing an empty list.
export const McpDiscoveryInitializing: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextMcpPending,
		},
	},
	play: async ({ canvasElement }) => {
		await openMcpSection(canvasElement);
	},
};

// A chat bound to an agent that has not pushed its first snapshot yet: the
// only thing to show is that discovery is underway.
export const McpDiscoveryInitializingBeforeFirstSnapshot: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: {
				...MockChatContextClean,
				resources: [],
				mcp_discovery: { phase: "pending", stale: false },
			},
		},
	},
	play: async ({ canvasElement }) => {
		await openMcpSection(canvasElement);
	},
};

// Discovery finished and no server was declared, so the section states that
// outcome rather than disappearing.
export const McpDiscoveryCompleteNoServers: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: MockChatContextMcpCompleteNoServers,
		},
	},
	play: async ({ canvasElement }) => {
		await openMcpSection(canvasElement);
	},
};

// A discovered server that exposes no tools stays listed with an explicit
// marker so it is not mistaken for a failed one.
export const McpServerWithoutTools: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: {
				...MockChatContextMcpComplete,
				resources: [
					...MockChatContextResourcesWithoutMcp,
					{
						source: "/home/coder/.mcp.json",
						kind: "mcp_config",
						size_bytes: 184,
						status: "ok",
					},
					{
						source: "linear",
						kind: "mcp_server",
						size_bytes: 0,
						status: "ok",
						tools: [],
					},
				],
			},
		},
	},
	play: async ({ canvasElement }) => {
		await openMcpSection(canvasElement);
	},
};

// An OK server carrying a non-fatal warning: it keeps its tools in the MCP
// list and the warning appears under Issues without marking it unavailable.
export const McpServerWithWarning: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: {
				...MockChatContextMcpComplete,
				resources: MockChatContextResourcesHealthy.map((resource) =>
					resource.kind === "mcp_server"
						? {
								...resource,
								error:
									"reconnect failed: connection refused; serving tools from the previous connection",
							}
						: resource,
				),
			},
		},
	},
	play: async ({ canvasElement }) => {
		await openMcpSection(canvasElement);
	},
};

// Failed sources: a semantically invalid .mcp.json and a server that could not
// be reached are both listed under Issues with their sanitized errors.
export const McpSourcesFailed: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: {
				...MockChatContextMcpComplete,
				resources: [
					...MockChatContextResourcesWithoutMcp,
					{
						source: "/home/coder/.mcp.json",
						kind: "mcp_config",
						size_bytes: 184,
						status: "invalid",
						error: 'server "github": command and url are mutually exclusive',
					},
					{
						source: "linear",
						kind: "mcp_server",
						size_bytes: 0,
						status: "unreadable",
						error: "initialize: dial tcp 127.0.0.1:8080: connection refused",
					},
				],
			},
		},
	},
	play: async ({ canvasElement }) => {
		await openMcpSection(canvasElement);
	},
};

// A failed server whose error embeds a long path with no break
// opportunities: the text must wrap inside the popover instead of widening
// it into a horizontal scroll.
export const McpLongError: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: {
				...MockChatContextMcpComplete,
				resources: [
					...MockChatContextResourcesWithoutMcp,
					{
						source: "longpath",
						kind: "mcp_server",
						size_bytes: 0,
						status: "unreadable",
						error: `connect "longpath": fork/exec /home/coder/${"deeplynesteddirectorysegment/".repeat(13)}mcpserverbinary: no such file or directory`,
					},
				],
			},
		},
	},
	play: async ({ canvasElement }) => {
		await openMcpSection(canvasElement);
	},
};

// The pinned MCP rows were published by a previous agent process, so their
// tools are withheld until the chat picks up the current agent's discovery.
export const McpDiscoveryStale: Story = {
	args: {
		usage: {
			usedTokens: 12_000,
			contextLimitTokens: 200_000,
			context: {
				...MockChatContextMcpComplete,
				mcp_discovery: { phase: "complete", stale: true },
			},
		},
	},
	play: async ({ canvasElement }) => {
		await openMcpSection(canvasElement);
	},
};

// Stands in for the chat detail refetch that the context_dirty watch event
// triggers: the indicator is prop-driven, so publishing the completed
// discovery swaps the context while the popover stays open.
const DiscoveryUpdateHarness: React.FC = () => {
	const [context, setContext] = useState<ChatContext>(
		MockChatContextMcpPending,
	);
	const usage: AgentContextUsage = {
		usedTokens: 12_000,
		contextLimitTokens: 200_000,
		context,
	};
	return (
		<div className="flex items-center gap-4">
			<ContextUsageIndicator usage={usage} />
			<Button
				size="xs"
				variant="outline"
				onClick={() => setContext(MockChatContextMcpComplete)}
			>
				Publish discovery
			</Button>
		</div>
	);
};

export const McpDiscoveryUpdatesInPlace: Story = {
	render: () => <DiscoveryUpdateHarness />,
	play: async ({ canvasElement }) => {
		await openMcpSection(canvasElement);
		// fireEvent sends no pointerdown, so the open popover does not
		// dismiss and re-renders with the published discovery instead.
		await fireEvent.click(
			within(canvasElement).getByRole("button", {
				name: "Publish discovery",
			}),
		);
	},
};
