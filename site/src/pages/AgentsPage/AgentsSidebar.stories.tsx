import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as TypesGen from "api/typesGenerated";
import type { Chat } from "api/typesGenerated";
import type { ModelSelectorOption } from "components/ai-elements";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { AgentsSidebar } from "./AgentsSidebar";

const defaultModelOptions: ModelSelectorOption[] = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
];

const defaultModelConfigs: TypesGen.ChatModelConfig[] = [
	{
		id: "config-openai-gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		display_name: "GPT-4o",
		enabled: true,
		is_default: false,
		context_limit: 200000,
		compression_threshold: 70,
		created_at: "2026-02-18T00:00:00.000Z",
		updated_at: "2026-02-18T00:00:00.000Z",
	},
];

const buildChat = (overrides: Partial<Chat> = {}): Chat => ({
	id: "chat-default",
	owner_id: "owner-1",
	title: "Agent",
	status: "completed",
	last_model_config_id: defaultModelConfigs[0].id,
	created_at: "2026-02-18T00:00:00.000Z",
	updated_at: "2026-02-18T00:00:00.000Z",
	archived: false,
	last_error: null,
	...overrides,
});

const agentsRouting = [
	{ path: "/agents/:agentId", useStoryElement: true },
	{ path: "/agents", useStoryElement: true },
] satisfies [
	{ path: string; useStoryElement: boolean },
	...{ path: string; useStoryElement: boolean }[],
];

const meta: Meta<typeof AgentsSidebar> = {
	title: "pages/AgentsPage/AgentsSidebar",
	component: AgentsSidebar,
	args: {
		chatErrorReasons: {},
		modelOptions: defaultModelOptions,
		modelConfigs: defaultModelConfigs,
		onArchiveAgent: fn(),
		onNewAgent: fn(),
		isCreating: false,
	},
	parameters: {
		layout: "fullscreen",
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
};

export default meta;
type Story = StoryObj<typeof AgentsSidebar>;

export const SearchFiltering: Story = {
	args: {
		chats: [
			buildChat({ id: "parent-1", title: "Parent planner" }),
			buildChat({
				id: "child-1",
				title: "Child executor",
				parent_chat_id: "parent-1",
				root_chat_id: "parent-1",
			}),
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByPlaceholderText("Search agents..."),
			"child",
		);
		await waitFor(() => {
			expect(canvas.getByText("Parent planner")).toBeInTheDocument();
			expect(canvas.getByText("Child executor")).toBeInTheDocument();
		});
	},
};

export const RunningDelegatedChat: Story = {
	args: {
		chats: [
			buildChat({ id: "root-1", title: "Root agent" }),
			buildChat({
				id: "child-running",
				title: "Running child",
				status: "running",
				parent_chat_id: "root-1",
				root_chat_id: "root-1",
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/child-running",
				pathParams: { agentId: "child-running" },
			},
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByTestId("agents-tree-executing-child-running"),
		).toBeInTheDocument();
	},
};

export const PendingDelegatedChat: Story = {
	args: {
		chats: [
			buildChat({ id: "root-pending", title: "Root agent" }),
			buildChat({
				id: "child-pending",
				title: "Pending child",
				status: "pending",
				parent_chat_id: "root-pending",
				root_chat_id: "root-pending",
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/child-pending",
				pathParams: { agentId: "child-pending" },
			},
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByTestId("agents-tree-executing-child-pending"),
		).toBeInTheDocument();
	},
};

export const ExpandCollapse: Story = {
	args: {
		chats: [
			buildChat({ id: "root-2", title: "Root for collapse" }),
			buildChat({
				id: "child-collapse",
				title: "Nested child",
				parent_chat_id: "root-2",
				root_chat_id: "root-2",
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/child-collapse",
				pathParams: { agentId: "child-collapse" },
			},
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = canvas.getByTestId("agents-tree-toggle-root-2");

		await expect(toggle).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Nested child")).toBeInTheDocument();

		await userEvent.click(toggle);
		await expect(toggle).toHaveAttribute("aria-expanded", "false");
		expect(canvas.queryByText("Nested child")).not.toBeInTheDocument();

		await userEvent.click(toggle);
		await expect(toggle).toHaveAttribute("aria-expanded", "true");
		expect(canvas.getByText("Nested child")).toBeInTheDocument();
	},
};

export const RunningChatPreservesSpinner: Story = {
	args: {
		chats: [
			buildChat({
				id: "root-running",
				title: "Running root agent",
				status: "running",
			}),
			buildChat({
				id: "child-of-running",
				title: "Child of running",
				parent_chat_id: "root-running",
				root_chat_id: "root-running",
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/child-of-running",
				pathParams: { agentId: "child-of-running" },
			},
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The root chat is running and has children, so a spinning
		// Loader2Icon should be rendered inside the icon wrapper.
		const node = canvas.getByTestId("agents-tree-node-root-running");
		const spinner = node.querySelector(".animate-spin");
		await expect(spinner).toBeInTheDocument();

		// The toggle button should exist (the node has children) but
		// must be invisible by default — it only appears on hover of
		// the icon area itself, not the whole row.
		const toggle = canvas.getByTestId("agents-tree-toggle-root-running");
		await expect(toggle).toBeInTheDocument();
		await expect(toggle.className).toMatch(/\binvisible\b/);
	},
};

export const ActiveChatAncestryExpanded: Story = {
	args: {
		chats: [
			buildChat({ id: "root-active", title: "Active root" }),
			buildChat({
				id: "child-active",
				title: "Active middle",
				parent_chat_id: "root-active",
				root_chat_id: "root-active",
			}),
			buildChat({
				id: "grandchild-active",
				title: "Active leaf",
				parent_chat_id: "child-active",
				root_chat_id: "root-active",
			}),
			buildChat({ id: "other-root", title: "Other root" }),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/grandchild-active",
				pathParams: { agentId: "grandchild-active" },
			},
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Active root")).toBeInTheDocument();
		await waitFor(() => {
			expect(canvas.getByText("Active middle")).toBeInTheDocument();
			expect(canvas.getByText("Active leaf")).toBeInTheDocument();
		});
		await expect(
			canvas.getByTestId("agents-tree-toggle-root-active"),
		).toHaveAttribute("aria-expanded", "true");
		await expect(
			canvas.getByTestId("agents-tree-toggle-child-active"),
		).toHaveAttribute("aria-expanded", "true");
	},
};
