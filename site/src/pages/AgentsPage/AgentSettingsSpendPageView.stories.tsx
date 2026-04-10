import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
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
		total_runtime_ms: 0,
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
		total_runtime_ms: 0,
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
	total_runtime_ms: 0,
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
			total_runtime_ms: 0,
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
			total_runtime_ms: 0,
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
			user_id: "user-3",
			username: "dave",
			name: "Dave Grohl",
			avatar_url: "",
			spend_limit_micros: 100_000_000,
		},
		{
			user_id: "user-4",
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

// Helper to build a mock usersQuery object that satisfies the view's
// PaginationResult & query shape.
function mockUsersQuery(
	opts: {
		data?: TypesGen.ChatCostUsersResponse;
		isLoading?: boolean;
		isFetching?: boolean;
		error?: unknown;
	} = {},
): PaginationResult & {
	data: TypesGen.ChatCostUsersResponse | undefined;
	isLoading: boolean;
	isFetching: boolean;
	error: unknown;
	refetch: () => unknown;
} {
	const data = opts.data;
	const isSuccess = data !== undefined && !opts.error;
	return {
		data,
		isLoading: opts.isLoading ?? false,
		isFetching: opts.isFetching ?? false,
		error: opts.error ?? null,
		refetch: fn(),
		isPlaceholderData: false,
		currentPage: 1,
		limit: 25,
		onPageChange: fn(),
		goToPreviousPage: fn(),
		goToNextPage: fn(),
		goToFirstPage: fn(),
		...(isSuccess
			? {
					isSuccess: true as const,
					hasNextPage: false,
					hasPreviousPage: false,
					totalRecords: data.count,
					totalPages: 1,
					currentOffsetStart: data.count === 0 ? 0 : 1,
					countIsCapped: false,
				}
			: {
					isSuccess: false as const,
					hasNextPage: false,
					hasPreviousPage: false,
					totalRecords: undefined,
					totalPages: undefined,
					currentOffsetStart: undefined,
					countIsCapped: false,
				}),
	};
}

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
	endDateIsExclusive: false,
	searchFilter: "",
	usersQuery: mockUsersQuery(),
	drillInUserId: null as string | null,
	drillInUser: null as TypesGen.User | null,
	isDrillInUserLoading: false,
	isDrillInUserError: false,
	drillInUserError: undefined as unknown,
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
		onDrillInUserRetry: fn(),
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
		usersQuery: mockUsersQuery({ data: mockUsersResponse }),
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
		usersQuery: mockUsersQuery({
			data: {
				start_date: "2026-02-10T00:00:00Z",
				end_date: "2026-03-12T00:00:00Z",
				count: 0,
				users: [],
			},
		}),
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
		drillInUserId: "user-1",
		drillInUser: mockUserProfile,
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
		drillInUserId: "user-1",
		drillInUser: mockUserProfile,
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

export const SpendDrillInLoading: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
		drillInUserId: "user-1",
		drillInUser: null,
		isDrillInUserLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("status", { name: "Loading user details" }),
		).toBeInTheDocument();
	},
};

export const SpendDrillInError: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
		drillInUserId: "user-1",
		drillInUser: null,
		isDrillInUserError: true,
		drillInUserError: new Error("User not found"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText("User not found");
		await expect(canvas.getByText("Retry")).toBeInTheDocument();
	},
};

export const SpendRefetchOverlay: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
		usersQuery: mockUsersQuery({
			data: mockUsersResponse,
			isFetching: true,
		}),
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

export const SpendConfigError: Story = {
	args: {
		configError: new Error("Network error: failed to fetch config"),
		usersQuery: mockUsersQuery({ data: mockUsersResponse }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The inline config error should be visible.
		await canvas.findByText("Network error: failed to fetch config");
		await expect(canvas.getByText("Retry")).toBeInTheDocument();

		// The usage table should still render despite the config error.
		await expect(canvas.getByText("Alice Liddell")).toBeInTheDocument();
		await expect(canvas.getByText("Bob Builder")).toBeInTheDocument();
	},
};

export const SpendUsersLoading: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
		usersQuery: mockUsersQuery({ isLoading: true }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Config sections should still render.
		await canvas.findByText("Default spend limit");

		// The loading spinner for users should be visible.
		await expect(
			await canvas.findByRole("status", { name: "Loading usage" }),
		).toBeInTheDocument();
	},
};

export const SpendUsersError: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
		usersQuery: mockUsersQuery({
			error: new Error("Failed to load usage data"),
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Config sections should still render.
		await canvas.findByText("Default spend limit");

		// The usage error should be visible with retry.
		await expect(
			canvas.getByText("Failed to load usage data"),
		).toBeInTheDocument();
		await expect(canvas.getByText("Retry")).toBeInTheDocument();
	},
};

export const SpendUserClickToDrillIn: Story = {
	args: {
		configData: mockConfigData,
		groupsData: mockGroups,
		usersQuery: mockUsersQuery({ data: mockUsersResponse }),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// Click the first user row.
		const row = await canvas.findByRole("button", {
			name: /Alice Liddell/,
		});
		await userEvent.click(row);

		expect(args.onSelectUser).toHaveBeenCalledWith(
			expect.objectContaining({ user_id: "user-1" }),
		);
	},
};
