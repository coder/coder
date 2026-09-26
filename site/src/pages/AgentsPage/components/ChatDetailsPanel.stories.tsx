import type { Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { fn, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { chat, chatCost } from "#/api/queries/chats";
import type { ChatContext, FeatureName } from "#/api/typesGenerated";
import {
	MockChat,
	MockChatContextClean,
	MockChatContextDirty,
} from "#/testHelpers/chatEntities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { ChatDetailsPanel } from "./ChatDetailsPanel";

const PanelFrame = (Story: FC) => (
	<div className="h-[760px] w-[360px] max-w-full border border-solid border-border-default">
		<Story />
	</div>
);
const meta: Meta<typeof ChatDetailsPanel> = {
	title: "pages/AgentsPage/ChatDetailsPanel",
	component: ChatDetailsPanel,
	decorators: [PanelFrame, withDashboardProvider],
	parameters: {
		features: ["aibridge"] satisfies FeatureName[],
		queries: [
			{
				key: chat(MockChat.id).queryKey,
				data: {
					...MockChat,
					summary:
						"Investigated the flaky CI job, traced it to a cache-layer race, and added a regression test.",
				},
			},
			{
				key: chatCost(MockChat.id).queryKey,
				data: {
					chat_id: MockChat.id,
					total_cost_micros: 1_250_000,
					request_count: 8,
					unpriced_request_count: 0,
				},
			},
		],
	},
	args: {
		chatId: MockChat.id,
		isVisible: true,
		usage: {
			usedTokens: 26_000,
			contextLimitTokens: 1_100_000,
			compressionThreshold: 70,
			context: MockChatContextClean,
		},
		onApplyContext: fn(),
	},
};
export default meta;
type Story = StoryObj<typeof ChatDetailsPanel>;

const expandResources: Story["play"] = async ({ canvasElement }) => {
	const canvas = within(canvasElement);
	for (const name of [/^Context/, /^Skills/, /^MCP servers/])
		await userEvent.click(canvas.getByRole("button", { name }));
};

export const WithSummary: Story = {};
export const ExpandedResources: Story = { play: expandResources };
export const NoSummary: Story = {
	parameters: {
		queries: [
			{ key: chat(MockChat.id).queryKey, data: MockChat },
			{
				key: chatCost(MockChat.id).queryKey,
				data: {
					chat_id: MockChat.id,
					total_cost_micros: 0,
					request_count: 0,
					unpriced_request_count: 0,
				},
			},
		],
	},
};
export const SubagentSummaryPending: Story = {
	parameters: {
		queries: [
			{
				key: chat(MockChat.id).queryKey,
				data: { ...MockChat, parent_chat_id: "parent-chat-id" },
			},
			{
				key: chatCost("parent-chat-id").queryKey,
				data: {
					chat_id: "parent-chat-id",
					total_cost_micros: 0,
					request_count: 0,
					unpriced_request_count: 0,
				},
			},
		],
	},
};
export const SubagentTreeCost: Story = {
	parameters: {
		queries: [
			{
				key: chat(MockChat.id).queryKey,
				data: {
					...MockChat,
					parent_chat_id: "parent-chat-id",
					root_chat_id: "root-chat-id",
				},
			},
			{
				key: chatCost("root-chat-id").queryKey,
				data: {
					chat_id: "root-chat-id",
					total_cost_micros: 1_250_000,
					request_count: 8,
					unpriced_request_count: 1,
				},
			},
		],
	},
};
export const ChatError: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockRejectedValue(
			new Error("Failed to load chat"),
		);
	},
};
export const Loading: Story = {
	parameters: { queries: [] },
	beforeEach: () => {
		spyOn(API.experimental, "getChat").mockImplementation(
			() => new Promise(() => {}),
		);
	},
};
export const GatewayUnavailable: Story = { parameters: { features: [] } };
export const Dirty: Story = {
	args: { usage: { ...meta.args?.usage, context: MockChatContextDirty } },
};
export const Applying: Story = {
	args: { ...Dirty.args, isApplyingContext: true },
};
export const ApplyError: Story = {
	args: {
		...Dirty.args,
		applyError: new Error(
			"Workspace context is temporarily unavailable. Try again.",
		),
	},
};
export const Applied: Story = { args: { applySuccess: true } };
export const Archived: Story = {
	args: { ...Dirty.args, onApplyContext: undefined },
};
export const SnapshotError: Story = {
	args: {
		usage: {
			...meta.args?.usage,
			context: {
				...MockChatContextClean,
				error: "Failed to read AGENTS.md: permission denied",
			},
		},
	},
	play: expandResources,
};
export const NoUsage: Story = { args: { usage: null }, play: expandResources };
export const NoUsageCollapsed: Story = { args: NoUsage.args };
export const UsageWithoutInventory: Story = {
	args: { usage: { ...meta.args?.usage, context: undefined } },
};
export const UsageWithoutContextLimit: Story = {
	args: { usage: { usedTokens: 26_000, context: MockChatContextClean } },
	play: expandResources,
};
export const ZeroUsage: Story = {
	args: { usage: { ...meta.args?.usage, usedTokens: 0 } },
	play: expandResources,
};
export const EmptyContext: Story = {
	args: { usage: { context: { ...MockChatContextClean, resources: [] } } },
	play: expandResources,
};
export const UsageWithoutTokenCounts: Story = {
	args: {
		usage: { contextLimitTokens: 200_000, context: MockChatContextClean },
	},
	play: expandResources,
};
export const CompactionDisabled: Story = {
	args: { usage: { ...meta.args?.usage, compressionThreshold: 100 } },
	play: expandResources,
};
export const ZeroThreshold: Story = {
	args: { usage: { ...meta.args?.usage, compressionThreshold: 0 } },
	play: expandResources,
};
export const EstimatedUsage: Story = {
	args: { usage: { ...meta.args?.usage, estimated: true } },
	play: expandResources,
};

const MockMultipleRoots: ChatContext = {
	...MockChatContextClean,
	resources: [
		...(MockChatContextClean.resources ?? []),
		{
			source: "/home/coder/project/site/AGENTS.md",
			kind: "instruction_file",
			size_bytes: 512,
			status: "ok",
		},
		{
			source: "/home/coder/project/.agents/skills/deploy",
			kind: "skill",
			size_bytes: 140,
			status: "ok",
			skill_name: "deploy",
			skill_description: "Deploy this project to staging.",
		},
	],
};
export const MultipleContextRoots: Story = {
	args: { usage: { ...meta.args?.usage, context: MockMultipleRoots } },
	play: expandResources,
};
const MockMcpContext: ChatContext = {
	...MockChatContextClean,
	resources: [
		...(MockChatContextClean.resources ?? []),
		{
			source: "/home/coder/project/.mcp.json",
			kind: "mcp_config",
			size_bytes: 256,
			status: "ok",
		},
		{
			source: "connected-without-tools",
			kind: "mcp_server",
			size_bytes: 0,
			status: "ok",
			tools: [],
		},
		{
			source: "unreachable",
			kind: "mcp_server",
			size_bytes: 0,
			status: "unreadable",
			error: "Connection refused: could not discover tools.",
		},
	],
};
export const MultipleMcpConfigs: Story = {
	args: { usage: { ...meta.args?.usage, context: MockMcpContext } },
	play: expandResources,
};
export const FailedMcpCollapsed: Story = { args: MultipleMcpConfigs.args };
export const WorkspaceConnected: Story = {
	args: { workspaceStatus: "connected" },
};
export const WorkspaceStopped: Story = {
	args: { ...MultipleMcpConfigs.args, workspaceStatus: "stopped" },
};
export const NarrowLongContent: Story = {
	decorators: [
		(Story) => (
			<div className="w-[280px] max-w-full">
				<Story />
			</div>
		),
	],
	args: {
		usage: {
			...meta.args?.usage,
			context: {
				...MockMultipleRoots,
				resources: [
					...(MockMultipleRoots.resources ?? []),
					{
						source:
							"/home/coder/project/very-long-directory-name-that-must-wrap-without-horizontal-scrolling/nested-directory/AGENTS.md",
						kind: "instruction_file",
						status: "ok",
						size_bytes: 16384,
					},
					{
						source:
							"server-with-a-very-long-name-that-must-not-overflow-the-panel",
						kind: "mcp_server",
						status: "ok",
						size_bytes: 128,
						tools: [
							{
								name: "tool_with_an_extremely_long_name_that_must_wrap",
								description:
									"A detailed tool description that remains readable without relying on a hover-only tooltip.",
							},
						],
					},
				],
			},
		},
	},
	play: expandResources,
};
export const EnlargedText: Story = {
	...NarrowLongContent,
	beforeEach: () => {
		const previous = document.documentElement.style.fontSize;
		document.documentElement.style.fontSize = "32px";
		return () => {
			document.documentElement.style.fontSize = previous;
		};
	},
};
export const LightResourceIssues: Story = {
	...NarrowLongContent,
	parameters: { themes: { themeOverride: "light" } },
};

export const KeyboardFocus: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		canvas.getByRole("button", { name: "Summary" }).focus();
		await userEvent.tab();
	},
};
