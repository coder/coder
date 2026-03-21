import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { userByNameKey } from "api/queries/users";
import type * as TypesGen from "api/typesGenerated";
import dayjs from "dayjs";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { SettingsPageContent } from "./SettingsPageContent";

// ── Usage mock helpers ─────────────────────────────────────────

const mockUsers: TypesGen.ChatCostUserRollup[] = [
	{
		user_id: "user-1",
		username: "alice",
		name: "Alice Liddell",
		avatar_url: "",
		total_cost_micros: 2_500_000,
		message_count: 42,
		chat_count: 5,
		total_input_tokens: 200_000,
		total_output_tokens: 300_000,
		total_cache_read_tokens: 10_000,
		total_cache_creation_tokens: 5_000,
	},
	{
		user_id: "user-2",
		username: "bob",
		name: "Bob Builder",
		avatar_url: "",
		total_cost_micros: 1_000_000,
		message_count: 18,
		chat_count: 3,
		total_input_tokens: 80_000,
		total_output_tokens: 120_000,
		total_cache_read_tokens: 4_000,
		total_cache_creation_tokens: 2_000,
	},
];

const mockUsersResponse: TypesGen.ChatCostUsersResponse = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	count: mockUsers.length,
	users: mockUsers,
};

const mockUserProfile: TypesGen.User = {
	id: "user-1",
	username: "alice",
	name: "Alice Liddell",
	email: "alice@example.com",
	avatar_url: "",
	created_at: "2025-01-01T00:00:00Z",
	updated_at: "2025-06-01T00:00:00Z",
	status: "active",
	organization_ids: [],
	roles: [],
	last_seen_at: "2026-03-11T10:00:00Z",
	login_type: "password",
};

const mockCostSummary: TypesGen.ChatCostSummary = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	total_cost_micros: 2_500_000,
	priced_message_count: 40,
	unpriced_message_count: 2,
	total_input_tokens: 200_000,
	total_output_tokens: 300_000,
	total_cache_read_tokens: 10_000,
	total_cache_creation_tokens: 5_000,
	by_model: [
		{
			model_config_id: "model-1",
			display_name: "GPT-4.1",
			provider: "OpenAI",
			model: "gpt-4.1",
			total_cost_micros: 2_000_000,
			message_count: 30,
			total_input_tokens: 150_000,
			total_output_tokens: 250_000,
			total_cache_read_tokens: 8_000,
			total_cache_creation_tokens: 4_000,
		},
	],
	by_chat: [
		{
			root_chat_id: "chat-1",
			chat_title: "Refactor auth module",
			total_cost_micros: 1_200_000,
			message_count: 15,
			total_input_tokens: 80_000,
			total_output_tokens: 120_000,
			total_cache_read_tokens: 3_000,
			total_cache_creation_tokens: 1_500,
		},
	],
};

/**
 * Set up spies for all usage-related API methods. The behaviour mocks
 * (system prompt, desktop, custom prompt) are still inherited from
 * the meta-level `beforeEach`.
 */
const setupUsageSpies = (opts?: {
	usersResponse?: TypesGen.ChatCostUsersResponse;
}) => {
	spyOn(API, "getChatCostUsers").mockResolvedValue(
		opts?.usersResponse ?? mockUsersResponse,
	);
	spyOn(API, "getUser").mockResolvedValue(mockUserProfile);
	spyOn(API, "getChatCostSummary").mockResolvedValue(mockCostSummary);
};

// ── Meta ───────────────────────────────────────────────────────

const meta = {
	title: "pages/AgentsPage/SettingsPageContent",
	component: SettingsPageContent,
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		activeSection: "behavior",
		canManageChatModelConfigs: false,
		canSetSystemPrompt: true,
		now: dayjs("2026-03-12T00:00:00Z"),
	},
	parameters: {
		user: MockUserOwner,
		layout: "fullscreen",
	},
	beforeEach: () => {
		spyOn(API, "getChatSystemPrompt").mockResolvedValue({
			system_prompt: "",
		});
		spyOn(API, "updateChatSystemPrompt").mockResolvedValue();
		spyOn(API, "getChatDesktopEnabled").mockResolvedValue({
			enable_desktop: false,
		});
		spyOn(API, "updateChatDesktopEnabled").mockResolvedValue();
		spyOn(API, "getUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
		spyOn(API, "updateUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
	},
} satisfies Meta<typeof SettingsPageContent>;

export default meta;
type Story = StoryObj<typeof SettingsPageContent>;

// ── Behavior tab stories ───────────────────────────────────────

export const DesktopSetting: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Virtual Desktop");
		await canvas.findByText(
			/Allow agents to use a virtual, graphical desktop/i,
		);
		await canvas.findByRole("switch", { name: "Enable" });
	},
};

export const TogglesDesktop: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Enable",
		});

		await userEvent.click(toggle);
		await waitFor(() => {
			expect(API.updateChatDesktopEnabled).toHaveBeenCalledWith({
				enable_desktop: true,
			});
		});
	},
};

// ── Usage tab stories ──────────────────────────────────────────

export const UsageUserList: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	beforeEach: () => {
		setupUsageSpies();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The section header should be visible.
		await canvas.findByText("Usage");

		// Both users should appear in the table.
		await expect(await canvas.findByText("Alice Liddell")).toBeInTheDocument();
		await expect(canvas.getByText("Bob Builder")).toBeInTheDocument();

		// Verify the search field is present.
		await expect(
			canvas.getByPlaceholderText("Search by name or username"),
		).toBeInTheDocument();
	},
};

export const UsageEmpty: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	beforeEach: () => {
		setupUsageSpies({
			usersResponse: {
				start_date: "2026-02-10T00:00:00Z",
				end_date: "2026-03-12T00:00:00Z",
				count: 0,
				users: [],
			},
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Usage");
		await expect(
			await canvas.findByText("No usage data for this period."),
		).toBeInTheDocument();
	},
};

export const UsageUserDrillIn: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	parameters: {
		queries: [{ key: userByNameKey("user-1"), data: mockUserProfile }],
	},
	beforeEach: () => {
		setupUsageSpies();
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Wait for the user list to load.
		await userEvent.click(await body.findByText("Alice Liddell"));

		// The detail view should show the user header with name
		// and username subtitle.
		await expect(await body.findByText("Alice Liddell")).toBeInTheDocument();
		await expect(body.getByText("@alice")).toBeInTheDocument();

		// The user profile was pre-seeded in the query cache via
		// parameters.queries, so the detail header should show the
		// user ID from that data.
		await expect(
			body.getByText(`User ID: ${mockUserProfile.id}`),
		).toBeInTheDocument();

		// The cost summary should have been fetched.
		await waitFor(() => {
			expect(API.getChatCostSummary).toHaveBeenCalled();
		});

		// The Back button should be visible.
		await expect(body.getByText("Back")).toBeInTheDocument();
	},
};

export const UsageUserDrillInAndBack: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	parameters: {
		queries: [{ key: userByNameKey("user-1"), data: mockUserProfile }],
	},
	beforeEach: () => {
		setupUsageSpies();
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click Alice's row to drill in.
		await userEvent.click(await body.findByText("Alice Liddell"));

		// Wait for the detail view to appear.
		await body.findByText("@alice");

		// Click Back to return to the list.
		await userEvent.click(body.getByText("Back"));

		// The user list should be visible again with both users.
		await expect(await body.findByText("Alice Liddell")).toBeInTheDocument();
		await expect(body.getByText("Bob Builder")).toBeInTheDocument();

		// The search field should be present, confirming we're
		// back on the list view.
		await expect(
			body.getByPlaceholderText("Search by name or username"),
		).toBeInTheDocument();
	},
};
