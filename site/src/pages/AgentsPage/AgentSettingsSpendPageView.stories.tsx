import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { AgentSettingsSpendPageView } from "./AgentSettingsSpendPageView";

// ── Mock data ──────────────────────────────────────────────────

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

const mockUserProfile = {
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
} as TypesGen.User;

const mockCostSummary = {
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
} as TypesGen.ChatCostSummary;

const mockConfigData = {
	spend_limit_micros: 50_000_000,
	period: "month",
	updated_at: "2026-03-01T00:00:00Z",
	unpriced_model_count: 0,
	overrides: [
		{
			user_id: "user-1",
			username: "alice",
			name: "Alice Liddell",
			avatar_url: "",
			spend_limit_micros: 100_000_000,
		},
		{
			user_id: "user-3",
			username: "charlie",
			name: "Charlie Chaplin",
			avatar_url: "",
			spend_limit_micros: 25_000_000,
		},
	],
	group_overrides: [
		{
			group_id: "group-1",
			group_name: "engineering",
			group_display_name: "Engineering",
			group_avatar_url: "",
			member_count: 12,
			spend_limit_micros: 75_000_000,
		},
	],
} as TypesGen.ChatUsageLimitConfigResponse;

const mockGroups = [
	{
		id: "group-1",
		name: "engineering",
		display_name: "Engineering",
		organization_id: "org-1",
		members: [],
		total_member_count: 12,
		avatar_url: "",
		quota_allowance: 0,
		source: "user",
		organization_name: "default",
		organization_display_name: "Default",
	},
	{
		id: "group-2",
		name: "design",
		display_name: "Design",
		organization_id: "org-1",
		members: [],
		total_member_count: 5,
		avatar_url: "",
		quota_allowance: 0,
		source: "user",
		organization_name: "default",
		organization_display_name: "Default",
	},
] as TypesGen.Group[];

const defaultDateRange = {
	startDate: new Date("2026-02-10T00:00:00Z"),
	endDate: new Date("2026-03-12T00:00:00Z"),
};

// Baseline props shared across stories. Only primitives and simple
// objects here to avoid the composeStory deep-merge hang.
const baseProps = {
	// Limits config.
	configData: undefined as TypesGen.ChatUsageLimitConfigResponse | undefined,
	isLoadingConfig: false,
	configError: null as Error | null,
	groupsData: undefined as TypesGen.Group[] | undefined,
	isLoadingGroups: false,
	groupsError: null as Error | null,
	isUpdatingConfig: false,
	updateConfigError: null as Error | null,
	isUpdateConfigSuccess: false,
	isUpsertingOverride: false,
	upsertOverrideError: null as Error | null,
	isDeletingOverride: false,
	deleteOverrideError: null as Error | null,
	isUpsertingGroupOverride: false,
	upsertGroupOverrideError: null as Error | null,
	isDeletingGroupOverride: false,
	deleteGroupOverrideError: null as Error | null,
	// Usage data.
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
	title: "pages/AgentsPage/AgentSettingsSpendPageView",
	component: AgentSettingsSpendPageView,
	args: {
		...baseProps,
		refetchConfig: fn(),
		onUpdateConfig: fn(),
		resetUpdateConfig: fn(),
		onUpsertOverride: fn(),
		onDeleteOverride: fn(),
		onUpsertGroupOverride: fn(),
		onDeleteGroupOverride: fn(),
		onDateRangeChange: fn(),
		onSearchFilterChange: fn(),
		onPageChange: fn(),
		onUsersRetry: fn(),
		onSelectedUserRetry: fn(),
		onClearSelectedUser: fn(),
		onSelectUser: fn(),
		onSummaryRetry: fn(),
	},
} satisfies Meta<typeof AgentSettingsSpendPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsSpendPageView>;

// ── Stories ────────────────────────────────────────────────────

export const SpendWithLimitsAndUsers: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
		usersData: mockUsersResponse,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The header and all three collapsible sections should render.
		await canvas.findByText("Spend management");
		await expect(canvas.getByText("Default spend limit")).toBeInTheDocument();
		await expect(canvas.getByText("Group limits")).toBeInTheDocument();
		await expect(canvas.getByText("Per-user spend")).toBeInTheDocument();

		// User table rows should be visible.
		await expect(await canvas.findByText("Alice Liddell")).toBeInTheDocument();
		await expect(canvas.getByText("Bob Builder")).toBeInTheDocument();

		// Search field should be present.
		await expect(
			canvas.getByPlaceholderText("Search by name or username"),
		).toBeInTheDocument();
	},
};

export const SpendUsersEmpty: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
		usersData: {
			start_date: "2026-02-10T00:00:00Z",
			end_date: "2026-03-12T00:00:00Z",
			count: 0,
			users: [],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Spend management");
		await expect(
			await canvas.findByText("No usage data for this period."),
		).toBeInTheDocument();
	},
};

export const SpendUserDrillIn: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
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

export const SpendUserDrillInAndBack: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
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

export const SpendRefetchOverlay: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
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

export const SpendConfigLoading: Story = {
	args: {
		isLoadingConfig: true,
	},
};
