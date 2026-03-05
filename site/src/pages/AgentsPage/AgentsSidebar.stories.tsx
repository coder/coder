import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
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

const oneWeekAgo = new Date(Date.now() - 7 * 24 * 60 * 60 * 1000).toISOString();

const buildChat = (overrides: Partial<Chat> = {}): Chat => ({
	id: "chat-default",
	owner_id: "owner-1",
	title: "Agent",
	status: "completed",
	last_model_config_id: defaultModelConfigs[0].id,
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
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
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		chatErrorReasons: {},
		modelOptions: defaultModelOptions,
		modelConfigs: defaultModelConfigs,
		onArchiveAgent: fn(),
		onUnarchiveAgent: fn(),
		onArchiveAndDeleteWorkspace: fn(),
		onNewAgent: fn(),
		isCreating: false,
	},
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
};

export default meta;
type Story = StoryObj<typeof AgentsSidebar>;

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

// When a root chat is idle but has a running child, the chevron
// should still be scoped to the icon area hover, not the full row.
export const IdleParentWithRunningChild: Story = {
	args: {
		chats: [
			buildChat({
				id: "idle-parent",
				title: "Idle parent agent",
				status: "waiting",
			}),
			buildChat({
				id: "running-child",
				title: "Running sub-agent",
				status: "running",
				parent_chat_id: "idle-parent",
				root_chat_id: "idle-parent",
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/running-child",
				pathParams: { agentId: "running-child" },
			},
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The parent's toggle should use icon-only hover scope because
		// its child is actively running.
		const toggle = canvas.getByTestId("agents-tree-toggle-idle-parent");
		await expect(toggle).toBeInTheDocument();
		await expect(toggle.className).toMatch(/\binvisible\b/);
		await expect(toggle.className).toContain("group-hover/icon:visible");
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

const todayTimestamp = new Date().toISOString();

export const ArchivedAgentsCollapsed: Story = {
	args: {
		chats: [
			buildChat({
				id: "active-1",
				title: "Active agent one",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "active-2",
				title: "Active agent two",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "archived-1",
				title: "Archived agent one",
				archived: true,
			}),
			buildChat({
				id: "archived-2",
				title: "Archived agent two",
				archived: true,
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("Active agent one")).toBeInTheDocument();
			expect(canvas.getByText("Active agent two")).toBeInTheDocument();
			expect(canvas.getByText("Archived (2)")).toBeInTheDocument();
		});
		expect(canvas.queryByText("Archived agent one")).not.toBeInTheDocument();
		expect(canvas.queryByText("Archived agent two")).not.toBeInTheDocument();
	},
};

export const ArchivedAgentsExpanded: Story = {
	args: {
		chats: [
			buildChat({
				id: "active-1",
				title: "Active agent one",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "active-2",
				title: "Active agent two",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "archived-1",
				title: "Archived agent one",
				archived: true,
			}),
			buildChat({
				id: "archived-2",
				title: "Archived agent two",
				archived: true,
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("Archived (2)")).toBeInTheDocument();
		});
		await userEvent.click(canvas.getByText("Archived (2)"));
		await waitFor(() => {
			expect(canvas.getByText("Archived agent one")).toBeInTheDocument();
			expect(canvas.getByText("Archived agent two")).toBeInTheDocument();
		});
	},
};

export const NoArchivedSection: Story = {
	args: {
		chats: [
			buildChat({
				id: "chat-a",
				title: "First active agent",
				updated_at: todayTimestamp,
			}),
			buildChat({
				id: "chat-b",
				title: "Second active agent",
				updated_at: todayTimestamp,
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("First active agent")).toBeInTheDocument();
			expect(canvas.getByText("Second active agent")).toBeInTheDocument();
		});
		expect(canvas.queryByText(/^Archived \(/)).not.toBeInTheDocument();
	},
};

export const ArchivingShowsSpinnerOnly: Story = {
	args: {
		chats: [
			buildChat({
				id: "archiving-chat",
				title: "Chat being archived",
				updated_at: todayTimestamp,
			}),
		],
		isArchiving: true,
		archivingChatId: "archiving-chat",
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
};

export const DefaultShowsTimestampHidesMenu: Story = {
	args: {
		chats: [
			buildChat({
				id: "default-chat",
				title: "Default state agent",
				updated_at: todayTimestamp,
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
};

export const WithDiffStats: Story = {
	args: {
		chats: [
			buildChat({
				id: "diff-both",
				title: "Agent with additions and deletions",
				updated_at: todayTimestamp,
				diff_status: {
					chat_id: "diff-both",
					url: "https://github.com/coder/coder/pull/1",
					changes_requested: false,
					additions: 42,
					deletions: 7,
					changed_files: 5,
				},
			}),
			buildChat({
				id: "diff-add-only",
				title: "Agent with additions only",
				updated_at: todayTimestamp,
				diff_status: {
					chat_id: "diff-add-only",
					url: "https://github.com/coder/coder/pull/2",
					changes_requested: false,
					additions: 120,
					deletions: 0,
					changed_files: 3,
				},
			}),
			buildChat({
				id: "diff-del-only",
				title: "Agent with deletions only",
				updated_at: todayTimestamp,
				diff_status: {
					chat_id: "diff-del-only",
					url: "https://github.com/coder/coder/pull/3",
					changes_requested: false,
					additions: 0,
					deletions: 35,
					changed_files: 2,
				},
			}),
			buildChat({
				id: "diff-none",
				title: "Agent with no diff changes",
				updated_at: todayTimestamp,
				diff_status: {
					chat_id: "diff-none",
					url: "https://github.com/coder/coder/pull/4",
					changes_requested: false,
					additions: 0,
					deletions: 0,
					changed_files: 0,
				},
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("+42")).toBeInTheDocument();
			expect(canvas.getByText("+120")).toBeInTheDocument();
		});
		// The deletions-only agent should show −35.
		const delOnlyNode = canvas.getByTestId("agents-tree-node-diff-del-only");
		expect(
			within(delOnlyNode).getByText("35", { exact: false }),
		).toBeInTheDocument();
		// The zero-change agent should NOT render any diff numbers.
		const noneNode = canvas.getByTestId("agents-tree-node-diff-none");
		expect(within(noneNode).queryByText("+")).not.toBeInTheDocument();
	},
};

export const WithDiffStatsLight: Story = {
	globals: {
		theme: "light",
	},
	args: {
		chats: [
			buildChat({
				id: "diff-both-light",
				title: "Agent with additions and deletions",
				updated_at: todayTimestamp,
				diff_status: {
					chat_id: "diff-both-light",
					url: "https://github.com/coder/coder/pull/1",
					changes_requested: false,
					additions: 42,
					deletions: 7,
					changed_files: 5,
				},
			}),
			buildChat({
				id: "diff-add-only-light",
				title: "Agent with additions only",
				updated_at: todayTimestamp,
				diff_status: {
					chat_id: "diff-add-only-light",
					url: "https://github.com/coder/coder/pull/2",
					changes_requested: false,
					additions: 120,
					deletions: 0,
					changed_files: 3,
				},
			}),
			buildChat({
				id: "diff-del-only-light",
				title: "Agent with deletions only",
				updated_at: todayTimestamp,
				diff_status: {
					chat_id: "diff-del-only-light",
					url: "https://github.com/coder/coder/pull/3",
					changes_requested: false,
					additions: 0,
					deletions: 35,
					changed_files: 2,
				},
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("+42")).toBeInTheDocument();
			expect(canvas.getByText("+120")).toBeInTheDocument();
		});
	},
};

export const ArchivedAgentUnarchiveOption: Story = {
	args: {
		chats: [
			buildChat({
				id: "archived-unarchive",
				title: "Archived agent with unarchive",
				archived: true,
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Expand archived section
		await waitFor(() => {
			expect(canvas.getByText("Archived (1)")).toBeInTheDocument();
		});
		await userEvent.click(canvas.getByText("Archived (1)"));
		await waitFor(() => {
			expect(
				canvas.getByText("Archived agent with unarchive"),
			).toBeInTheDocument();
		});
		// Open the dropdown menu for the archived agent
		const trigger = canvas.getByLabelText(
			"Open actions for Archived agent with unarchive",
		);
		await userEvent.click(trigger);
		// Verify "Unarchive agent" is shown instead of "Archive agent"
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Unarchive agent")).toBeInTheDocument();
		});
		const body = within(document.body);
		expect(body.queryByText("Archive agent")).not.toBeInTheDocument();
		expect(
			body.queryByText("Archive & delete workspace"),
		).not.toBeInTheDocument();
	},
};
