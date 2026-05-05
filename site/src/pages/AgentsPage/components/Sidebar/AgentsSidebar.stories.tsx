import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocation } from "react-router";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { userChatProviderConfigsKey } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import type { Chat } from "#/api/typesGenerated";
import { MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import type { ModelSelectorOption } from "../ChatElements";
import { AgentsSidebar } from "./AgentsSidebar";

// Probe element used by the archived-filter preservation story to surface the
// search string of whatever child route the sidebar's NavLink ends up at.
const ChildSearchProbe = () => {
	const location = useLocation();
	return <div data-testid="child-search">{location.search}</div>;
};

// Probe element used by the settings-link preservation story to surface the
// state.from value passed when navigating to settings.
const SettingsStateProbe = () => {
	const location = useLocation();
	const from = (location.state as { from?: string })?.from ?? "";
	return <div data-testid="settings-state-from">{from}</div>;
};

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
	organization_id: "test-org-id",
	owner_id: "owner-1",
	title: "Agent",
	status: "completed",
	last_model_config_id: defaultModelConfigs[0].id,
	mcp_server_ids: [],
	labels: {},
	created_at: oneWeekAgo,
	updated_at: oneWeekAgo,
	archived: false,
	pin_order: 0,
	has_unread: false,
	client_type: "ui",
	children: [],
	...overrides,
});

const agentsRouting = [
	{ path: "/agents/:agentId", useStoryElement: true },
	{ path: "/agents", useStoryElement: true },
] satisfies [
	{ path: string; useStoryElement: boolean },
	...{ path: string; useStoryElement: boolean }[],
];

const settingsRouting = [
	{ path: "/agents/settings/:section", useStoryElement: true },
	{ path: "/agents/settings", useStoryElement: true },
	...agentsRouting,
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
		onPinAgent: fn(),
		onUnpinAgent: fn(),
		onRenameTitle: fn(() => Promise.resolve()),
		onBeforeNewAgent: fn(),
		isCreating: false,
		regeneratingTitleChatIds: [],
		archivedFilter: "active" as const,
		onArchivedFilterChange: fn(),
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
			buildChat({
				id: "root-1",
				title: "Root agent",
				children: [
					buildChat({
						id: "child-running",
						title: "Running child",
						status: "running",
						parent_chat_id: "root-1",
						root_chat_id: "root-1",
					}),
				],
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
			buildChat({
				id: "root-pending",
				title: "Root agent",
				children: [
					buildChat({
						id: "child-pending",
						title: "Pending child",
						status: "pending",
						parent_chat_id: "root-pending",
						root_chat_id: "root-pending",
					}),
				],
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
			buildChat({
				id: "root-2",
				title: "Root for collapse",
				children: [
					buildChat({
						id: "child-collapse",
						title: "Nested child",
						parent_chat_id: "root-2",
						root_chat_id: "root-2",
					}),
				],
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
				children: [
					buildChat({
						id: "child-of-running",
						title: "Child of running",
						parent_chat_id: "root-running",
						root_chat_id: "root-running",
					}),
				],
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
				children: [
					buildChat({
						id: "running-child",
						title: "Running sub-agent",
						status: "running",
						parent_chat_id: "idle-parent",
						root_chat_id: "idle-parent",
					}),
				],
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
	// This story uses the flat-sibling shape (parent_chat_id on each
	// entry) rather than embedded children. It intentionally
	// exercises the defensive fallback loop in buildChatTree for
	// stale cache data from before root-only pagination landed.
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

export const MixedCacheDoesNotDuplicateChild: Story = {
	// Simulates the rollout window where a stale cache entry for a child
	// chat still appears as a flat sibling in the paginated list while
	// the same child is also embedded under its parent. Without the
	// guard in buildChatTree (`if (!parentById.has(chat.id))` around
	// setting the parent link to undefined), the flat entry would
	// overwrite the embedded parent link and the defensive fallback
	// would re-add the child to its parent's children list, producing
	// a React duplicate-key warning and double-render.
	args: {
		chats: [
			buildChat({
				id: "mixed-root",
				title: "Mixed root",
				children: [
					buildChat({
						id: "mixed-child",
						title: "Mixed child",
						parent_chat_id: "mixed-root",
						root_chat_id: "mixed-root",
					}),
				],
			}),
			// Stale flat entry for the same child still present in the
			// cache. It must not cause a duplicate render.
			buildChat({
				id: "mixed-child",
				title: "Mixed child",
				parent_chat_id: "mixed-root",
				root_chat_id: "mixed-root",
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/mixed-child",
				pathParams: { agentId: "mixed-child" },
			},
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Mixed root")).toBeInTheDocument();
		await waitFor(() => {
			expect(canvas.getAllByText("Mixed child")).toHaveLength(1);
		});
	},
};

// Use a fixed offset so the value always falls in the "Today" bucket
// without embedding a literal date that drifts across calendar days.
const recentTimestamp = new Date(Date.now() - 60_000).toISOString();

export const RenameChatAvailableDuringRegeneration: Story = {
	args: {
		chats: [
			buildChat({
				id: "regenerating-chat",
				title: "Regenerating agent",
				updated_at: recentTimestamp,
			}),
			buildChat({
				id: "idle-chat",
				title: "Idle agent",
				updated_at: recentTimestamp,
			}),
		],
		regeneratingTitleChatIds: ["regenerating-chat"],
		onProposeTitle: fn(async () => "Proposed replacement"),
		onRenameTitle: fn(async () => {}),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await expect(canvas.getByText("Regenerating agent")).toHaveAttribute(
			"aria-busy",
			"true",
		);

		await userEvent.click(
			canvas.getByRole("button", {
				name: "Open actions for Regenerating agent",
			}),
		);
		await expect(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		).toBeInTheDocument();

		await userEvent.keyboard("{Escape}");
		await waitFor(() => {
			expect(
				body.queryByRole("menuitem", { name: "Rename chat" }),
			).not.toBeInTheDocument();
		});

		await userEvent.click(
			canvas.getByRole("button", {
				name: "Open actions for Idle agent",
			}),
		);
		await expect(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		).toBeInTheDocument();
	},
};

export const RenameChatSubmitsNewTitle: Story = {
	args: {
		chats: [
			buildChat({
				id: "rename-target",
				title: "Original title",
				updated_at: recentTimestamp,
			}),
		],
		onRenameTitle: fn(() => Promise.resolve()),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getByRole("button", {
				name: "Open actions for Original title",
			}),
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		);

		const input = await body.findByRole<HTMLInputElement>("textbox", {
			name: "Chat title",
		});
		await waitFor(() => {
			expect(input).toHaveValue("Original title");
			expect(input.selectionStart).toBe(0);
			expect(input.selectionEnd).toBe("Original title".length);
		});

		await userEvent.clear(input);
		await userEvent.type(input, "Renamed title", { delay: null });
		await waitFor(() => {
			expect(input).toHaveValue("Renamed title");
		});
		await userEvent.click(body.getByRole("button", { name: "Save" }));

		await waitFor(() => {
			expect(args.onRenameTitle).toHaveBeenCalledWith(
				"rename-target",
				"Renamed title",
			);
		});
		await waitFor(() => {
			expect(
				body.queryByRole("heading", { name: "Rename chat" }),
			).not.toBeInTheDocument();
		});
	},
};

export const CancellingRenameDialogKeepsTitle: Story = {
	args: {
		chats: [
			buildChat({
				id: "rename-cancel",
				title: "Keep me",
				updated_at: recentTimestamp,
			}),
		],
		onRenameTitle: fn(() => Promise.resolve()),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getByRole("button", {
				name: "Open actions for Keep me",
			}),
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		);

		const input = await body.findByRole<HTMLInputElement>("textbox", {
			name: "Chat title",
		});
		await userEvent.clear(input);
		await userEvent.type(input, "Discarded edit");
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		expect(args.onRenameTitle).not.toHaveBeenCalled();
		expect(canvas.getByText("Keep me")).toBeInTheDocument();
	},
};

const animatedGeneratedTitle =
	"AI suggested title for a complex workspace migration with focused follow up tasks";

export const RenameChatGenerateFillsInput: Story = {
	args: {
		chats: [
			buildChat({
				id: "rename-generate",
				title: "Old title",
				updated_at: recentTimestamp,
			}),
		],
		onProposeTitle: fn(async () => animatedGeneratedTitle),
		onRenameTitle: fn(() => Promise.resolve()),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getByRole("button", {
				name: "Open actions for Old title",
			}),
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		);

		const input = await body.findByRole<HTMLInputElement>("textbox", {
			name: "Chat title",
		});

		await userEvent.click(body.getByRole("button", { name: "Generate" }));
		await waitFor(
			() => {
				const value = input.value;
				expect(value.length).toBeGreaterThan(0);
				expect(animatedGeneratedTitle.startsWith(value)).toBe(true);
				expect(value).not.toBe(animatedGeneratedTitle);
				expect(body.getByRole("button", { name: "Generate" })).toBeDisabled();
				expect(body.getByRole("button", { name: "Save" })).toBeDisabled();
			},
			{ timeout: 2_000 },
		);
		await waitFor(
			() => {
				expect(input).toHaveValue(animatedGeneratedTitle);
			},
			{ timeout: 4_000 },
		);
		expect(body.getByRole("button", { name: "Generate" })).toBeEnabled();
		expect(body.getByRole("button", { name: "Save" })).toBeEnabled();
		expect(args.onProposeTitle).toHaveBeenCalledWith("rename-generate");
		expect(args.onRenameTitle).not.toHaveBeenCalled();
	},
};

export const RenameChatGenerateErrorSurfacesAlert: Story = {
	args: {
		chats: [
			buildChat({
				id: "rename-generate-error",
				title: "Original title",
				updated_at: recentTimestamp,
			}),
		],
		onProposeTitle: fn(async () => {
			throw new Error("Proposal provider is temporarily unavailable.");
		}),
		onRenameTitle: fn(() => Promise.resolve()),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getByRole("button", {
				name: "Open actions for Original title",
			}),
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		);

		const input = await body.findByRole<HTMLInputElement>("textbox", {
			name: "Chat title",
		});

		await userEvent.click(body.getByRole("button", { name: "Generate" }));

		const alert = await body.findByRole("alert");
		expect(alert).toHaveTextContent(
			"Proposal provider is temporarily unavailable.",
		);
		await waitFor(() => {
			expect(input).toHaveAttribute("aria-invalid", "true");
		});
		expect(input).toHaveValue("Original title");
		expect(body.getByRole("button", { name: "Generate" })).toBeEnabled();
	},
};

export const RenameChatCancelAfterGenerateRestoresTitle: Story = {
	args: {
		chats: [
			buildChat({
				id: "rename-generate-cancel",
				title: "Keep this one",
				updated_at: recentTimestamp,
			}),
		],
		onProposeTitle: fn(async () => animatedGeneratedTitle),
		onRenameTitle: fn(() => Promise.resolve()),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getByRole("button", {
				name: "Open actions for Keep this one",
			}),
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		);

		const input = await body.findByRole<HTMLInputElement>("textbox", {
			name: "Chat title",
		});
		await userEvent.click(body.getByRole("button", { name: "Generate" }));
		await waitFor(
			() => {
				const value = input.value;
				expect(value.length).toBeGreaterThan(0);
				expect(animatedGeneratedTitle.startsWith(value)).toBe(true);
				expect(value).not.toBe(animatedGeneratedTitle);
				expect(body.getByRole("button", { name: "Generate" })).toBeDisabled();
				expect(body.getByRole("button", { name: "Save" })).toBeDisabled();
			},
			{ timeout: 2_000 },
		);

		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() => {
			expect(
				body.queryByRole("heading", { name: "Rename chat" }),
			).not.toBeInTheDocument();
		});
		expect(args.onRenameTitle).not.toHaveBeenCalled();
		expect(canvas.getByText("Keep this one")).toBeInTheDocument();

		await userEvent.click(
			canvas.getByRole("button", {
				name: "Open actions for Keep this one",
			}),
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		);
		const reopenedInput = await body.findByRole<HTMLInputElement>("textbox", {
			name: "Chat title",
		});
		expect(reopenedInput).toHaveValue("Keep this one");
		await new Promise((resolve) => setTimeout(resolve, 300));
		expect(reopenedInput).toHaveValue("Keep this one");
	},
};

export const RenameChatGenerateLateResponseDoesNotClobberOtherChat: Story = {
	args: {
		chats: [
			buildChat({
				id: "rename-generate-a",
				title: "Chat A",
				updated_at: recentTimestamp,
			}),
			buildChat({
				id: "rename-generate-b",
				title: "Chat B",
				updated_at: recentTimestamp,
			}),
		],
		onProposeTitle: fn((chatId: string) => {
			if (chatId === "rename-generate-a") {
				return new Promise<string>((resolve) => {
					setTimeout(() => resolve("Late suggestion for A"), 150);
				});
			}
			return Promise.resolve(`Proposal for ${chatId}`);
		}),
		onRenameTitle: fn(() => Promise.resolve()),
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getByRole("button", { name: "Open actions for Chat A" }),
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		);
		await body.findByRole<HTMLInputElement>("textbox", {
			name: "Chat title",
		});
		await userEvent.click(body.getByRole("button", { name: "Generate" }));

		await userEvent.click(body.getByRole("button", { name: "Cancel" }));

		await userEvent.click(
			await canvas.findByRole("button", {
				name: "Open actions for Chat B",
			}),
		);
		await userEvent.click(
			await body.findByRole("menuitem", { name: "Rename chat" }),
		);
		const inputB = await body.findByRole<HTMLInputElement>("textbox", {
			name: "Chat title",
		});
		expect(inputB).toHaveValue("Chat B");
		await userEvent.clear(inputB);
		await userEvent.paste("User edit for B");

		await new Promise((resolve) => setTimeout(resolve, 250));
		expect(inputB).toHaveValue("User edit for B");
		expect(body.queryByRole("alert")).not.toBeInTheDocument();
	},
};

export const RenameChatGenerateLateResponseDoesNotClobberSameChatReopen: Story =
	{
		args: {
			chats: [
				buildChat({
					id: "rename-generate-same",
					title: "Chat same",
					updated_at: recentTimestamp,
				}),
			],
			onProposeTitle: fn(() => {
				return new Promise<string>((resolve) => {
					setTimeout(() => resolve("Late suggestion"), 150);
				});
			}),
			onRenameTitle: fn(() => Promise.resolve()),
		},
		parameters: {
			reactRouter: reactRouterParameters({
				location: { path: "/agents" },
				routing: agentsRouting,
			}),
		},
		play: async ({ canvasElement }) => {
			const canvas = within(canvasElement);
			const body = within(document.body);

			await userEvent.click(
				canvas.getByRole("button", { name: "Open actions for Chat same" }),
			);
			await userEvent.click(
				await body.findByRole("menuitem", { name: "Rename chat" }),
			);
			await body.findByRole<HTMLInputElement>("textbox", {
				name: "Chat title",
			});
			await userEvent.click(body.getByRole("button", { name: "Generate" }));

			await userEvent.click(body.getByRole("button", { name: "Cancel" }));

			await userEvent.click(
				await canvas.findByRole("button", {
					name: "Open actions for Chat same",
				}),
			);
			await userEvent.click(
				await body.findByRole("menuitem", { name: "Rename chat" }),
			);
			const input = await body.findByRole<HTMLInputElement>("textbox", {
				name: "Chat title",
			});
			expect(input).toHaveValue("Chat same");
			await userEvent.clear(input);
			await userEvent.paste("User edit");

			await new Promise((resolve) => setTimeout(resolve, 250));
			expect(input).toHaveValue("User edit");
			expect(body.queryByRole("alert")).not.toBeInTheDocument();
		},
	};

export const ActiveFilterShowsActiveAgents: Story = {
	args: {
		chats: [
			buildChat({
				id: "active-1",
				title: "Active agent one",
				updated_at: recentTimestamp,
			}),
			buildChat({
				id: "active-2",
				title: "Active agent two",
				updated_at: recentTimestamp,
			}),
		],
		archivedFilter: "active",
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
		});
		expect(canvas.getByLabelText("Filter agents")).toBeInTheDocument();
	},
};

export const ArchivedFilterShowsArchivedAgents: Story = {
	args: {
		chats: [
			buildChat({
				id: "archived-1",
				title: "Archived agent one",
				archived: true,
				updated_at: recentTimestamp,
			}),
			buildChat({
				id: "archived-2",
				title: "Archived agent two",
				archived: true,
				updated_at: recentTimestamp,
			}),
		],
		archivedFilter: "archived",
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
			expect(canvas.getByText("Archived agent one")).toBeInTheDocument();
			expect(canvas.getByText("Archived agent two")).toBeInTheDocument();
		});
		expect(canvas.getByLabelText("Filter agents")).toBeInTheDocument();
	},
};

export const PreservesArchivedFilterOnChatNavigation: Story = {
	args: {
		chats: [
			buildChat({
				id: "archived-nav-1",
				title: "Archived nav target",
				archived: true,
				updated_at: recentTimestamp,
			}),
		],
		archivedFilter: "archived",
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents",
				searchParams: { archived: "archived" },
			},
			routing: [
				{ path: "/agents", useStoryElement: true },
				{ path: "/agents/:agentId", element: <ChildSearchProbe /> },
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const link = await canvas.findByRole("link", {
			name: /Archived nav target/,
		});
		await userEvent.click(link);
		await waitFor(() => {
			expect(canvas.getByTestId("child-search")).toHaveTextContent(
				"archived=archived",
			);
		});
	},
};

export const NoArchivedSection: Story = {
	args: {
		chats: [
			buildChat({
				id: "chat-a",
				title: "First active agent",
				updated_at: recentTimestamp,
			}),
			buildChat({
				id: "chat-b",
				title: "Second active agent",
				updated_at: recentTimestamp,
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
				updated_at: recentTimestamp,
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
				updated_at: recentTimestamp,
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
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "diff-both",
					url: "https://github.com/coder/coder/pull/1",
					pull_request_title: "",
					pull_request_draft: false,
					changes_requested: false,
					additions: 42,
					deletions: 7,
					changed_files: 5,
				},
			}),
			buildChat({
				id: "diff-add-only",
				title: "Agent with additions only",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "diff-add-only",
					url: "https://github.com/coder/coder/pull/2",
					pull_request_title: "",
					pull_request_draft: false,
					changes_requested: false,
					additions: 120,
					deletions: 0,
					changed_files: 3,
				},
			}),
			buildChat({
				id: "diff-del-only",
				title: "Agent with deletions only",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "diff-del-only",
					url: "https://github.com/coder/coder/pull/3",
					pull_request_title: "",
					pull_request_draft: false,
					changes_requested: false,
					additions: 0,
					deletions: 35,
					changed_files: 2,
				},
			}),
			buildChat({
				id: "diff-none",
				title: "Agent with no diff changes",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "diff-none",
					url: "https://github.com/coder/coder/pull/4",
					pull_request_title: "",
					pull_request_draft: false,
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
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "diff-both-light",
					url: "https://github.com/coder/coder/pull/1",
					pull_request_title: "",
					pull_request_draft: false,
					changes_requested: false,
					additions: 42,
					deletions: 7,
					changed_files: 5,
				},
			}),
			buildChat({
				id: "diff-add-only-light",
				title: "Agent with additions only",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "diff-add-only-light",
					url: "https://github.com/coder/coder/pull/2",
					pull_request_title: "",
					pull_request_draft: false,
					changes_requested: false,
					additions: 120,
					deletions: 0,
					changed_files: 3,
				},
			}),
			buildChat({
				id: "diff-del-only-light",
				title: "Agent with deletions only",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "diff-del-only-light",
					url: "https://github.com/coder/coder/pull/3",
					pull_request_title: "",
					pull_request_draft: false,
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

export const WithPRStateIcons: Story = {
	args: {
		chats: [
			buildChat({
				id: "pr-open",
				title: "Open pull request",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "pr-open",
					url: "https://github.com/coder/coder/pull/100",
					pull_request_state: "open",
					pull_request_title: "feat: add new feature",
					pull_request_draft: false,
					changes_requested: false,
					additions: 50,
					deletions: 10,
					changed_files: 4,
				},
			}),
			buildChat({
				id: "pr-draft",
				title: "Draft pull request",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "pr-draft",
					url: "https://github.com/coder/coder/pull/101",
					pull_request_state: "open",
					pull_request_title: "wip: draft changes",
					pull_request_draft: true,
					changes_requested: false,
					additions: 20,
					deletions: 5,
					changed_files: 2,
				},
			}),
			buildChat({
				id: "pr-merged",
				title: "Merged pull request",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "pr-merged",
					url: "https://github.com/coder/coder/pull/102",
					pull_request_state: "merged",
					pull_request_title: "feat: completed feature",
					pull_request_draft: false,
					changes_requested: false,
					additions: 200,
					deletions: 80,
					changed_files: 12,
				},
			}),
			buildChat({
				id: "pr-closed",
				title: "Closed pull request",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "pr-closed",
					url: "https://github.com/coder/coder/pull/103",
					pull_request_state: "closed",
					pull_request_title: "fix: abandoned approach",
					pull_request_draft: false,
					changes_requested: false,
					additions: 15,
					deletions: 3,
					changed_files: 1,
				},
			}),
			buildChat({
				id: "pr-no-state",
				title: "No PR state (branch only)",
				updated_at: recentTimestamp,
				diff_status: {
					chat_id: "pr-no-state",
					url: "https://github.com/coder/coder/tree/my-branch",
					pull_request_title: "",
					pull_request_draft: false,
					changes_requested: false,
					additions: 10,
					deletions: 2,
					changed_files: 1,
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
};

export const WithUnreadChats: Story = {
	args: {
		chats: [
			buildChat({
				id: "unread-1",
				title: "Unread chat with new activity",
				has_unread: true,
				updated_at: recentTimestamp,
			}),
			buildChat({
				id: "read-1",
				title: "Already read chat",
				has_unread: false,
				updated_at: recentTimestamp,
			}),
			buildChat({
				id: "unread-2",
				title: "Another unread chat",
				has_unread: true,
				status: "running",
				updated_at: recentTimestamp,
			}),
			buildChat({
				id: "unread-active",
				title: "Unread but currently viewed",
				has_unread: true,
				updated_at: recentTimestamp,
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents/unread-active",
				pathParams: { agentId: "unread-active" },
			},
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			// Unread indicators should be visible for unread chats
			// that are NOT the active chat.
			expect(
				canvas.getByTestId("unread-indicator-unread-1"),
			).toBeInTheDocument();
			expect(
				canvas.getByTestId("unread-indicator-unread-2"),
			).toBeInTheDocument();
		});
		// Read chat should not have an unread indicator.
		expect(
			canvas.queryByTestId("unread-indicator-read-1"),
		).not.toBeInTheDocument();
		// Unread chat that IS the active chat should not show
		// the indicator — the user is already viewing it.
		expect(
			canvas.queryByTestId("unread-indicator-unread-active"),
		).not.toBeInTheDocument();
	},
};

export const ArchivedAgentUnarchiveOption: Story = {
	args: {
		chats: [
			buildChat({
				id: "archived-unarchive",
				title: "Archived agent with unarchive",
				archived: true,
				updated_at: recentTimestamp,
			}),
		],
		archivedFilter: "archived",
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

export const PinnedChatsSection: Story = {
	args: {
		chats: [
			buildChat({
				id: "pinned-1",
				title: "My pinned agent",
				updated_at: recentTimestamp,
				pin_order: 1,
			}),
			buildChat({
				id: "unpinned-1",
				title: "Regular agent one",
				updated_at: recentTimestamp,
			}),
			buildChat({
				id: "unpinned-2",
				title: "Regular agent two",
				updated_at: recentTimestamp,
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
			expect(canvas.getByText("Pinned")).toBeInTheDocument();
			expect(canvas.getByText("My pinned agent")).toBeInTheDocument();
		});

		// Pinned chat must not appear again under the "Today" time group.
		const allPinnedLinks = canvas.getAllByText("My pinned agent");
		expect(allPinnedLinks).toHaveLength(1);

		// Unpinned chats appear under their time group, not Pinned.
		expect(canvas.getByText("Today")).toBeInTheDocument();
		expect(canvas.getByText("Regular agent one")).toBeInTheDocument();
	},
};

export const PinUnpinContextMenu: Story = {
	args: {
		chats: [
			buildChat({
				id: "pin-test",
				title: "Agent to pin",
				updated_at: recentTimestamp,
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("Agent to pin")).toBeInTheDocument();
		});
		const trigger = canvas.getByLabelText("Open actions for Agent to pin");
		await userEvent.click(trigger);
		await waitFor(() => {
			const body = within(document.body);
			expect(body.getByText("Pin agent")).toBeInTheDocument();
		});
		// Click Pin agent and verify callback.
		const body = within(document.body);
		await userEvent.click(body.getByText("Pin agent"));
		expect(args.onPinAgent).toHaveBeenCalledWith("pin-test");
	},
};

export const UnpinContextMenu: Story = {
	args: {
		chats: [
			buildChat({
				id: "unpin-test",
				title: "Agent to unpin",
				updated_at: recentTimestamp,
				pin_order: 1,
			}),
		],
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: agentsRouting,
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText("Agent to unpin")).toBeInTheDocument();
		});
		const trigger = canvas.getByLabelText("Open actions for Agent to unpin");
		await userEvent.click(trigger);
		const body = within(document.body);
		await waitFor(() => {
			expect(body.getByText("Unpin agent")).toBeInTheDocument();
		});
		await userEvent.click(body.getByText("Unpin agent"));
		expect(args.onUnpinAgent).toHaveBeenCalledWith("unpin-test");
	},
};

export const FilterOnPinnedHeader: Story = {
	args: {
		chats: [
			buildChat({
				id: "pinned-filter",
				title: "Pinned Chat",
				updated_at: recentTimestamp,
				pin_order: 1,
			}),
			buildChat({
				id: "unpinned-filter",
				title: "Unpinned Chat",
				updated_at: recentTimestamp,
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
			expect(canvas.getByText("Pinned")).toBeInTheDocument();
			expect(canvas.getByLabelText("Filter agents")).toBeInTheDocument();
		});
	},
};

export const FilterOnTimeGroupNoPins: Story = {
	args: {
		chats: [
			buildChat({
				id: "today-only",
				title: "Today Chat",
				updated_at: recentTimestamp,
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
			expect(canvas.getByText("Today")).toBeInTheDocument();
			expect(canvas.getByLabelText("Filter agents")).toBeInTheDocument();
		});
	},
};

export const SettingsAPIKeysAdmin: Story = {
	args: {
		chats: [],
		isAdmin: true,
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents/settings/api-keys" },
			routing: settingsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "Secrets (API keys)" }),
		).toBeInTheDocument();
	},
};

export const SettingsAPIKeysNonAdmin: Story = {
	args: {
		chats: [],
		isAdmin: false,
	},
	parameters: {
		queries: [
			{
				key: userChatProviderConfigsKey,
				data: [
					{
						provider_id: "prov-1",
						provider: "openai",
						display_name: "OpenAI",
						has_user_api_key: false,
						has_central_api_key_fallback: false,
					},
				],
			},
		],
		reactRouter: reactRouterParameters({
			location: { path: "/agents/settings/api-keys" },
			routing: settingsRouting,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: "Secrets (API keys)" }),
		).toBeInTheDocument();
	},
};

export const PreservesArchivedFilterOnSettingsNavigation: Story = {
	args: {
		chats: [
			buildChat({
				id: "archived-settings-1",
				title: "Archived settings target",
				archived: true,
				updated_at: recentTimestamp,
			}),
		],
		archivedFilter: "archived",
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: "/agents",
				searchParams: { archived: "archived" },
			},
			routing: [
				{
					path: "/agents/settings",
					element: <SettingsStateProbe />,
				},
				...agentsRouting,
			],
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const settingsLink = await canvas.findByLabelText("Settings");
		await userEvent.click(settingsLink);
		await waitFor(() => {
			const fromValue =
				canvas.getByTestId("settings-state-from").textContent ?? "";
			expect(fromValue).toContain("/agents");
			expect(fromValue).toContain("archived=archived");
		});
	},
};
