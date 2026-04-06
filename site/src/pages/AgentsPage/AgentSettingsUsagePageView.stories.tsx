import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { AgentSettingsUsagePageView } from "./AgentSettingsUsagePageView";

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
	has_ai_seat: false,
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

const defaultDateRange = {
	startDate: new Date("2026-02-10T00:00:00Z"),
	endDate: new Date("2026-03-12T00:00:00Z"),
};

const baseProps = {
	dateRange: defaultDateRange,
	hasExplicitDateRange: false,
	searchFilter: "",
	page: 1,
	pageSize: 25,
	offset: 0,
	isUsersLoading: false,
	isUsersFetching: false,
	usersError: undefined as unknown,
	selectedUserId: null as string | null,
	selectedUser: null as TypesGen.User | null,
	isSelectedUserLoading: false,
	isSelectedUserError: false,
	selectedUserError: undefined as unknown,
	summaryData: undefined as TypesGen.ChatCostSummary | undefined,
	isSummaryLoading: false,
	summaryError: undefined as unknown,
};

const meta = {
	title: "pages/AgentsPage/AgentSettingsUsagePageView",
	component: AgentSettingsUsagePageView,
	args: {
		...baseProps,
		onDateRangeChange: fn(),
		onSearchFilterChange: fn(),
		onPageChange: fn(),
		onUsersRetry: fn(),
		onSelectedUserRetry: fn(),
		onClearSelectedUser: fn(),
		onSelectUser: fn(),
		onSummaryRetry: fn(),
	},
} satisfies Meta<typeof AgentSettingsUsagePageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsUsagePageView>;

export const UsageUserList: Story = {
	args: {
		usersData: mockUsersResponse,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Usage");
		await expect(await canvas.findByText("Alice Liddell")).toBeInTheDocument();
		await expect(canvas.getByText("Bob Builder")).toBeInTheDocument();
		await expect(
			canvas.getByPlaceholderText("Search by name or username"),
		).toBeInTheDocument();
	},
};

export const UsageDateFilter: Story = {
	args: {
		usersData: mockUsersResponse,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Usage");

		// The date range picker trigger should be visible.
		const datePickerTrigger = await canvas.findByRole("button", {
			name: /Feb.*Mar/,
		});
		expect(datePickerTrigger).toBeInTheDocument();
	},
};

export const UsageDateFilterRefetchOverlay: Story = {
	args: {
		usersData: mockUsersResponse,
		isUsersFetching: true,
		isUsersLoading: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Table data should be visible behind the overlay.
		await canvas.findByText("Alice Liddell");

		// The refetch overlay spinner should be shown.
		await expect(
			await canvas.findByRole("status", { name: "Refreshing usage" }),
		).toBeInTheDocument();
	},
};

export const UsageEmpty: Story = {
	args: {
		usersData: {
			start_date: "2026-02-10T00:00:00Z",
			end_date: "2026-03-12T00:00:00Z",
			count: 0,
			users: [],
		},
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
		selectedUserId: "user-1",
		selectedUser: mockUserProfile,
		summaryData: mockCostSummary,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Detail view shows user info.
		await canvas.findByText(`User ID: ${mockUserProfile.id}`);
		await expect(canvas.getByText("Alice Liddell")).toBeInTheDocument();
		await expect(canvas.getByText("@alice")).toBeInTheDocument();

		// The Back button should be visible.
		await expect(canvas.getByText("Back")).toBeInTheDocument();
	},
};

export const UsageUserDrillInAndBack: Story = {
	args: {
		selectedUserId: "user-1",
		selectedUser: mockUserProfile,
		summaryData: mockCostSummary,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await canvas.findByText(`User ID: ${mockUserProfile.id}`);

		// Click Back.
		await userEvent.click(canvas.getByText("Back"));

		// The onClearSelectedUser callback should have been called.
		expect(args.onClearSelectedUser).toHaveBeenCalled();
	},
};
