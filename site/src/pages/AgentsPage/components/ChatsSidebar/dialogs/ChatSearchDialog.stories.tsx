import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { CHAT_SEARCH_LIMIT } from "#/api/queries/chats";
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
const longMockChats: Chat[] = [
	{
		...mockChat,
		id: "chat-long-1",
		title:
			"Review this PR and respond to every inline comment with detailed notes about selected row behavior in Table.tsx",
		last_turn_summary:
			"Posted review on PR #25069 with 10 inline comments covering 1 P2 issue, 4 P3s, and 2 observations.",
		updated_at: "2026-05-20T09:30:00.000Z",
		has_unread: false,
	},
];
const cappedMockChats: Chat[] = Array.from(
	{ length: CHAT_SEARCH_LIMIT },
	(_, index) => ({
		...mockChat,
		id: `chat-${index + 1}`,
		title: `Fix capped search result ${index + 1}`,
		has_unread: false,
		diff_status: undefined,
	}),
);

const meta: Meta<typeof ChatSearchDialog> = {
	title: "pages/AgentsPage/ChatSearchDialog",
	component: ChatSearchDialog,
	args: {
		open: true,
		onOpenChange: fn(),
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
	play: async () => {
		const body = within(document.body);
		await userEvent.type(
			body.getByRole("combobox", { name: "Search chats" }),
			"Fix",
		);
		await expect(await body.findByText(/results/i)).toBeInTheDocument();
		await waitFor(() => {
			expect(
				document.body.querySelectorAll('[data-slot="skeleton"]').length,
			).toBeGreaterThan(0);
		});
	},
};

export const Results: Story = {
	play: async () => {
		const body = within(document.body);
		await userEvent.type(
			body.getByRole("combobox", { name: "Search chats" }),
			"Fix",
		);
		await waitFor(() => {
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: CHAT_SEARCH_LIMIT,
				q: 'title:"Fix"',
			});
		});
		await expect(
			await body.findByText("Fix race condition in auth middleware"),
		).toBeInTheDocument();
	},
};

export const RefreshingResults: Story = {
	beforeEach: () => {
		let requestCount = 0;
		spyOn(API.experimental, "getChats").mockImplementation(() => {
			requestCount += 1;
			if (requestCount === 1) {
				return Promise.resolve(mockChats);
			}
			return new Promise<Chat[]>((_resolve) => {
				// Keep request pending to show the refresh indicator with stale results.
			});
		});
	},
	play: async () => {
		const body = within(document.body);
		const searchInput = body.getByRole("combobox", { name: "Search chats" });

		await userEvent.type(searchInput, "Fix");
		await expect(
			await body.findByText("Fix race condition in auth middleware"),
		).toBeInTheDocument();

		await userEvent.clear(searchInput);
		await userEvent.type(searchInput, "review");

		await waitFor(() => {
			expect(API.experimental.getChats).toHaveBeenCalledTimes(2);
		});
		await expect(body.getByLabelText("Searching chats")).toBeInTheDocument();
		await expect(
			body.getByText("Fix race condition in auth middleware"),
		).toBeInTheDocument();
	},
};

export const LongResults: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockResolvedValue(longMockChats);
	},
	play: async () => {
		const body = within(document.body);
		await userEvent.type(
			body.getByRole("combobox", { name: "Search chats" }),
			"review",
		);
		await waitFor(() => {
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: CHAT_SEARCH_LIMIT,
				q: 'title:"review"',
			});
		});

		const result = await body.findByRole("option", {
			name: /Review this PR and respond/i,
		});
		const summary = await body.findByText(/Posted review on PR #25069/i);
		const dialog = result.closest('[role="dialog"]');
		if (!dialog) {
			throw new Error("Expected search result to render in a dialog");
		}

		const dialogRight = Math.ceil(dialog.getBoundingClientRect().right);
		expect(Math.ceil(result.getBoundingClientRect().right)).toBeLessThanOrEqual(
			dialogRight,
		);
		expect(
			Math.ceil(summary.getBoundingClientRect().right),
		).toBeLessThanOrEqual(dialogRight);
	},
};

export const CappedResults: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockResolvedValue(cappedMockChats);
	},
	play: async () => {
		const body = within(document.body);
		await userEvent.type(
			body.getByRole("combobox", { name: "Search chats" }),
			"Fix",
		);
		await waitFor(() => {
			expect(API.experimental.getChats).toHaveBeenCalledWith({
				limit: CHAT_SEARCH_LIMIT,
				q: 'title:"Fix"',
			});
		});
		await expect(
			await body.findByText(
				(_content, element) =>
					element?.textContent?.replace(/\s+/g, " ").trim() ===
					`Showing first ${CHAT_SEARCH_LIMIT} results.`,
			),
		).toBeInTheDocument();
	},
};

export const KeyboardNavigation: Story = {
	play: async ({ args }) => {
		const body = within(document.body);
		const searchInput = body.getByRole("combobox", { name: "Search chats" });

		await userEvent.type(searchInput, "Fix");

		const firstResult = await body.findByRole("option", {
			name: /Fix race condition in auth middleware/i,
		});
		const secondResult = await body.findByRole("option", {
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

		await userEvent.keyboard("{ArrowUp}");
		await expect(secondResult).toHaveAttribute("aria-selected", "true");

		await userEvent.keyboard("{ArrowUp}");
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
	play: async () => {
		const body = within(document.body);
		await userEvent.type(
			body.getByRole("combobox", { name: "Search chats" }),
			"none",
		);
		await expect(
			await body.findByText("No matching chats"),
		).toBeInTheDocument();
	},
};

export const ErrorState: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChats").mockRejectedValue(
			new Error("Bad filter"),
		);
	},
	play: async () => {
		const body = within(document.body);
		await userEvent.type(
			body.getByRole("combobox", { name: "Search chats" }),
			"title:",
		);
		await expect(await body.findByRole("alert")).toBeInTheDocument();
	},
};
