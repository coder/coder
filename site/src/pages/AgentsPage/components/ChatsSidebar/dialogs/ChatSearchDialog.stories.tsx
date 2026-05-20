import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type { Chat } from "#/api/typesGenerated";
import { ChatSearchDialog } from "./ChatSearchDialog";

const mockDiffStatus: NonNullable<Chat["diff_status"]> = {
	chat_id: "chat-1",
	url: "https://github.com/coder/coder/pull/25391",
	pull_request_state: "open",
	pull_request_title: "Fix race condition",
	pull_request_draft: false,
	changes_requested: false,
	additions: 143,
	deletions: 125,
	changed_files: 8,
};

const mockChat: Chat = {
	id: "chat-1",
	organization_id: "org-1",
	owner_id: "owner-1",
	owner_username: "jaayden",
	title: "Fix race condition in auth middleware",
	status: "completed",
	last_model_config_id: "model-1",
	mcp_server_ids: [],
	labels: {},
	last_turn_summary: "Added migration script",
	created_at: "2026-05-20T05:00:00.000Z",
	updated_at: "2026-05-20T07:30:00.000Z",
	archived: false,
	pin_order: 0,
	has_unread: true,
	client_type: "ui",
	children: [],
	diff_status: mockDiffStatus,
};

const mockChats: Chat[] = [
	mockChat,
	{
		...mockChat,
		id: "chat-2",
		title: "Fix flaky workspace search story",
		last_turn_summary: "Updated keyboard interactions",
		updated_at: "2026-05-20T08:45:00.000Z",
		has_unread: false,
		diff_status: {
			...mockDiffStatus,
			chat_id: "chat-2",
			pull_request_title: "Fix flaky story",
			additions: 48,
			deletions: 12,
			changed_files: 3,
		},
	},
];

const meta: Meta<typeof ChatSearchDialog> = {
	title: "pages/AgentsPage/ChatSearchDialog",
	component: ChatSearchDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		onBeforeNewAgent: fn(),
		location: {
			pathname: "/agents",
			search: "",
			hash: "",
			state: null,
			key: "default",
		},
	},
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/agents" },
			routing: [
				{ path: "/agents", useStoryElement: true },
				{ path: "/agents/:agentId", useStoryElement: true },
				{ path: "/agents/settings", useStoryElement: true },
			],
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockResolvedValue(mockChats);
	},
};

export default meta;
type Story = StoryObj<typeof ChatSearchDialog>;

export const EmptyState: Story = {};

export const LoadingState: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockImplementation(
			() =>
				new Promise<Chat[]>((_resolve) => {
					// Keep request pending to hold loading skeleton.
				}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByRole("combobox", { name: "Search chats" }),
			"Fix",
		);
		await expect(await canvas.findByText(/results/i)).toBeInTheDocument();
		await waitFor(() => {
			expect(
				canvasElement.querySelectorAll('[data-slot="skeleton"]').length,
			).toBeGreaterThan(0);
		});
	},
};

export const Results: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByRole("combobox", { name: "Search chats" }),
			"Fix",
		);
		await waitFor(() => {
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: 50,
				q: 'title:"Fix"',
			});
		});
		await expect(
			await canvas.findByText("Fix race condition in auth middleware"),
		).toBeInTheDocument();
	},
};

export const KeyboardNavigation: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const searchInput = canvas.getByRole("combobox", { name: "Search chats" });

		await userEvent.type(searchInput, "Fix");

		const firstResult = await canvas.findByRole("option", {
			name: /Fix race condition in auth middleware/i,
		});
		const secondResult = await canvas.findByRole("option", {
			name: /Fix flaky workspace search story/i,
		});
		const resultsViewport = firstResult.closest(
			"[data-radix-scroll-area-viewport]",
		);
		if (!resultsViewport) {
			throw new Error("Expected search results to render in a scroll viewport");
		}

		await expect(resultsViewport).toHaveAttribute("tabindex", "-1");
		await expect(firstResult).toHaveAttribute("tabindex", "-1");
		await expect(secondResult).toHaveAttribute("tabindex", "-1");

		await userEvent.keyboard("{ArrowDown}");
		await expect(firstResult).toHaveAttribute("aria-selected", "true");

		await userEvent.keyboard("{ArrowDown}");
		await expect(secondResult).toHaveAttribute("aria-selected", "true");

		await userEvent.keyboard("{ArrowUp}");
		await expect(firstResult).toHaveAttribute("aria-selected", "true");

		await userEvent.keyboard("{Enter}");
		await waitFor(() => {
			expect(args.onOpenChange).toHaveBeenCalledWith(false);
		});
	},
};

export const NoResults: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockResolvedValue([]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByRole("combobox", { name: "Search chats" }),
			"none",
		);
		await expect(
			await canvas.findByText("No matching chats"),
		).toBeInTheDocument();
	},
};

export const ErrorState: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockRejectedValue(
			new Error("Bad filter"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByRole("combobox", { name: "Search chats" }),
			"title:",
		);
		await expect(await canvas.findByRole("alert")).toBeInTheDocument();
	},
};
