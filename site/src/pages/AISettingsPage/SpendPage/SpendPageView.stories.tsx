import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import type { PaginationResult } from "#/components/PaginationWidget/PaginationContainer";
import { SpendPageView } from "./SpendPageView";

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
	unpriced_messages_having_usage_count: 2,
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

const defaultDateRange = {
	startDate: new Date("2026-02-10T00:00:00Z"),
	endDate: new Date("2026-03-12T00:00:00Z"),
};

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

const baseProps = {
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
	title: "pages/AISettingsPage/SpendPage/SpendPageView",
	component: SpendPageView,
	// TODO: Stories in this file fail when pixel runs their play functions. Fix them and remove the exclude.
	parameters: { pixel: { exclude: true } },
	args: {
		...baseProps,
		onDateRangeChange: fn(),
		onSearchFilterChange: fn(),
		onDrillInUserRetry: fn(),
		onClearSelectedUser: fn(),
		onSelectUser: fn(),
		onSummaryRetry: fn(),
	},
} satisfies Meta<typeof SpendPageView>;

export default meta;
type Story = StoryObj<typeof SpendPageView>;

export const SpendUsersEmpty: Story = {
	args: {
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

		await canvas.findByText("AI spend usage");
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"As of v2.36, AI Governance Cost Control replaces Coder Agents Cost Control.",
		);
		await expect(
			canvas.getByRole("link", { name: /Read more here/ }),
		).toHaveAttribute(
			"href",
			expect.stringContaining("/ai-coder/ai-gateway/cost-controls"),
		);
		await expect(
			await canvas.findByText("No usage data for this period."),
		).toBeInTheDocument();
	},
};

export const SpendUserDrillIn: Story = {
	args: {
		drillInUserId: "user-1",
		drillInUser: mockUserProfile,
		summaryData: mockCostSummary,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText(`User ID: ${mockUserProfile.id}`);
		await expect(canvas.getByText("Alice Liddell")).toBeInTheDocument();
		await expect(canvas.getByText("@alice")).toBeInTheDocument();

		await expect(canvas.getByText("Back")).toBeInTheDocument();
	},
};

export const SpendUserDrillInAndBack: Story = {
	args: {
		drillInUserId: "user-1",
		drillInUser: mockUserProfile,
		summaryData: mockCostSummary,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		await canvas.findByText(`User ID: ${mockUserProfile.id}`);

		await userEvent.click(canvas.getByText("Back"));

		expect(args.onClearSelectedUser).toHaveBeenCalled();
	},
};

export const SpendDrillInLoading: Story = {
	args: {
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
		usersQuery: mockUsersQuery({
			data: mockUsersResponse,
			isFetching: true,
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Alice Liddell");

		await expect(
			await canvas.findByRole("status", { name: "Refreshing usage" }),
		).toBeInTheDocument();
	},
};

export const SpendUsersLoading: Story = {
	args: {
		usersQuery: mockUsersQuery({ isLoading: true }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			await canvas.findByRole("status", { name: "Loading usage" }),
		).toBeInTheDocument();
	},
};

export const SpendUsersError: Story = {
	args: {
		usersQuery: mockUsersQuery({
			error: new Error("Failed to load usage data"),
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText("Failed to load usage data"),
		).toBeInTheDocument();
		await expect(canvas.getByText("Retry")).toBeInTheDocument();
	},
};

export const SpendUserClickToDrillIn: Story = {
	args: {
		usersQuery: mockUsersQuery({ data: mockUsersResponse }),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		const row = await canvas.findByRole("button", {
			name: /^View details for Alice Liddell/,
		});
		await userEvent.click(row);

		expect(args.onSelectUser).toHaveBeenCalledWith(
			expect.objectContaining({ user_id: "user-1" }),
		);
	},
};
