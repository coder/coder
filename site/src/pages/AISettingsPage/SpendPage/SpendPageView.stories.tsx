import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { mockPaginationResultBase } from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockAIGatewaySpendUser,
	MockAIGatewaySpendUserSummary,
	MockUserMember,
} from "#/testHelpers/entities";
import { SpendPageView, type SpendUsersQuery } from "./SpendPageView";

const mockUsers: TypesGen.AIGatewaySpendUser[] = [
	MockAIGatewaySpendUser,
	{
		...MockAIGatewaySpendUser,
		id: "user-2",
		username: "alice",
		name: "Alice Liddell",
		avatar_url: "",
		total_cost_micros: 1_000_000,
		request_count: 18,
		session_count: 3,
		input_tokens: 80_000,
		output_tokens: 120_000,
		cache_read_input_tokens: 4_000,
		cache_write_input_tokens: 2_000,
	},
];

const mockUsersResponse: TypesGen.AIGatewaySpendUsersResponse = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	count: mockUsers.length,
	users: mockUsers,
};

const mockUserProfile: TypesGen.User = {
	...MockUserMember,
	id: MockAIGatewaySpendUser.id,
	username: MockAIGatewaySpendUser.username,
	name: MockAIGatewaySpendUser.name ?? "",
	avatar_url: MockAIGatewaySpendUser.avatar_url ?? "",
};

const defaultDateRange = {
	startDate: new Date("2026-02-10T00:00:00Z"),
	endDate: new Date("2026-03-12T00:00:00Z"),
};

function mockUsersQuery(
	opts: {
		data?: TypesGen.AIGatewaySpendUsersResponse;
		isLoading?: boolean;
		isFetching?: boolean;
		error?: unknown;
	} = {},
): SpendUsersQuery {
	const data = opts.data;
	const isSuccess = data !== undefined && !opts.error;
	return {
		...mockPaginationResultBase,
		data,
		isLoading: opts.isLoading ?? false,
		isFetching: opts.isFetching ?? false,
		error: opts.error ?? null,
		refetch: fn(),
		isPlaceholderData: false,
		...(isSuccess
			? {
					isSuccess: true as const,
					totalRecords: data.count,
					totalPages: 1,
					currentOffsetStart: data.count === 0 ? 0 : 1,
				}
			: {
					isSuccess: false as const,
					hasNextPage: false as const,
					hasPreviousPage: false as const,
					totalRecords: undefined,
					totalPages: undefined,
					currentOffsetStart: undefined,
					countIsCapped: false as const,
				}),
	};
}

const meta = {
	title: "pages/AISettingsPage/SpendPage/SpendPageView",
	component: SpendPageView,
	args: {
		isEntitled: true,
		isEnabled: true,
		dateRange: defaultDateRange,
		endDateIsExclusive: false,
		searchFilter: "",
		usersQuery: mockUsersQuery({ data: mockUsersResponse }),
		drillInUserId: null,
		drillInUser: null,
		isDrillInUserLoading: false,
		drillInUserError: undefined,
		summaryData: undefined,
		isSummaryLoading: false,
		summaryError: undefined,
		onDateRangeChange: fn(),
		onSearchFilterChange: fn(),
		onDrillInUserRetry: fn(),
		onClearSelectedUser: fn(),
		onSummaryRetry: fn(),
	},
} satisfies Meta<typeof SpendPageView>;

export default meta;
type Story = StoryObj<typeof SpendPageView>;

export const Paywall: Story = {
	args: {
		isEntitled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("link", { name: /Start trial for free/ }),
		).toBeVisible();
		await expect(canvas.queryByText("AI spend")).not.toBeInTheDocument();
	},
};

export const NotEnabled: Story = {
	args: {
		isEnabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(
				"AI Gateway is included in your license, but not set up yet.",
			),
		).toBeVisible();
	},
};

export const Loading: Story = {
	args: {
		usersQuery: mockUsersQuery({ isLoading: true }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("status", { name: "Loading spend" }),
		).toBeVisible();
	},
};

export const Empty: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: { ...mockUsersResponse, count: 0, users: [] },
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("AI spend")).toBeVisible();
		await expect(
			canvas.getByText("No AI Gateway spend for this period."),
		).toBeVisible();
	},
};

export const Users: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const table = canvas.getByRole("table", { name: "Spend by user" });
		expect(within(table).getAllByRole("row")).toHaveLength(3);
		expect(within(table).queryByRole("button")).not.toBeInTheDocument();
		await expect(within(table).getByText("$2.50")).toBeVisible();
		await expect(within(table).getByText("$1.00")).toBeVisible();
		await expect(within(table).getByText("300,000")).toBeVisible();
		expect(within(table).queryByText(/unpriced/)).not.toBeInTheDocument();
		expect(canvas.queryByRole("alert")).not.toBeInTheDocument();

		const detailsLink = within(table).getByRole("link", {
			name: "Alice Liddell",
		});
		const detailsUrl = new URL(
			detailsLink.getAttribute("href") ?? "",
			"http://localhost",
		);
		expect(detailsUrl.searchParams.get("user")).toBe("user-2");
	},
};

export const UsersWithUnpricedRequests: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: {
				...mockUsersResponse,
				users: [{ ...MockAIGatewaySpendUser, unpriced_request_count: 3 }],
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("3 unpriced")).toBeVisible();
	},
};

export const UsersClampedToRetention: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: { ...mockUsersResponse, start_date: "2026-02-26T12:00:00Z" },
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			/Showing spend since .*Feb 26, 2026/,
		);
		await expect(
			canvas.getByRole("link", { name: "data retention period" }),
		).toBeVisible();
	},
};

export const UsersOutsideRetention: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: {
				...mockUsersResponse,
				start_date: "2026-03-12T00:00:00Z",
				count: 0,
				users: [],
			},
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"No AI Gateway spend is retained for the selected period.",
		);
	},
};

export const UsersSearchNoMatch: Story = {
	args: {
		searchFilter: "nobody",
		usersQuery: mockUsersQuery({
			data: { ...mockUsersResponse, count: 0, users: [] },
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("No users match this search.")).toBeVisible();
	},
};

export const Refreshing: Story = {
	args: {
		usersQuery: mockUsersQuery({ data: mockUsersResponse, isFetching: true }),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Alice Liddell")).toBeVisible();
		await expect(
			canvas.getByRole("status", { name: "Refreshing spend" }),
		).toBeVisible();
	},
};

export const UsersError: Story = {
	args: {
		usersQuery: mockUsersQuery({
			error: new Error("Failed to load spend data"),
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Failed to load spend data")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		expect(args.usersQuery.refetch).toHaveBeenCalled();
	},
};

export const UsersRefetchError: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: mockUsersResponse,
			error: new Error("Failed to refresh spend data"),
		}),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const table = canvas.getByRole("table", { name: "Spend by user" });
		await expect(within(table).getByText("$2.50")).toBeVisible();
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"Failed to refresh spend data",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		expect(args.usersQuery.refetch).toHaveBeenCalled();
	},
};

export const SearchTyping: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.type(
			canvas.getByRole("textbox", { name: "Search spend by name or username" }),
			"a",
		);
		expect(args.onSearchFilterChange).toHaveBeenCalledWith("a");
	},
};

export const DrillInLoading: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		isDrillInUserLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("status", { name: "Loading user details" }),
		).toBeVisible();
	},
};

export const DrillInUserError: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUserError: new Error("User not found"),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("User not found")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		expect(args.onDrillInUserRetry).toHaveBeenCalled();
	},
};

export const DrillInSummaryLoading: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		isSummaryLoading: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("@bob")).toBeVisible();
		await expect(
			canvas.getByRole("status", { name: "Loading spend details" }),
		).toBeVisible();
	},
};

export const DrillIn: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryData: MockAIGatewaySpendUserSummary,
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("The Builder, Bob")).toBeVisible();
		const sessionsLink = canvas.getByRole("link", { name: "View sessions" });
		const sessionsUrl = new URL(
			sessionsLink.getAttribute("href") ?? "",
			"http://localhost",
		);
		expect(sessionsUrl.pathname).toBe("/ai-gateway/sessions");
		expect(sessionsUrl.searchParams.get("filter")).toBe(
			`initiator:${MockAIGatewaySpendUser.id} started_after:"2026-02-10T00:00:00Z" started_before:"2026-03-12T00:00:00Z"`,
		);

		await expect(
			canvas.getByRole("heading", { name: "By model" }),
		).toBeVisible();
		const byModel = canvas.getByRole("table", { name: "Spend by model" });
		await expect(within(byModel).getByText("claude-opus-4-6")).toBeVisible();
		await expect(within(byModel).getByText("anthropic-main")).toBeVisible();
		await expect(within(byModel).getByText("$2.00")).toBeVisible();

		await expect(
			canvas.getByRole("heading", { name: "By client" }),
		).toBeVisible();
		const byClient = canvas.getByRole("table", { name: "Spend by client" });
		await expect(within(byClient).getByText("Claude Code")).toBeVisible();
		await expect(within(byClient).getByText("$1.80")).toBeVisible();

		await expect(canvas.queryByRole("note")).not.toBeInTheDocument();

		await userEvent.click(canvas.getByRole("button", { name: "Back" }));
		expect(args.onClearSelectedUser).toHaveBeenCalled();
	},
};

export const DrillInWithUnpricedRequests: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryData: {
			...MockAIGatewaySpendUserSummary,
			unpriced_request_count: 1,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("note")).toHaveTextContent(
			"Cost is unavailable for 1 request. The total excludes that usage.",
		);
	},
};

export const DrillInClampedToRetention: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryData: {
			...MockAIGatewaySpendUserSummary,
			start_date: "2026-02-26T12:00:00Z",
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			/Showing spend since .*Feb 26, 2026/,
		);
	},
};

export const DrillInTruncatedBreakdowns: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryData: {
			...MockAIGatewaySpendUserSummary,
			model_count: 137,
			client_count: 2,
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Showing the 2 most expensive of 137 models."),
		).toBeVisible();
		expect(
			canvas.queryByText(/most expensive of .* clients/),
		).not.toBeInTheDocument();
	},
};

export const DrillInEmpty: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryData: {
			...MockAIGatewaySpendUserSummary,
			total_cost_micros: 0,
			request_count: 0,
			unpriced_request_count: 0,
			session_count: 0,
			input_tokens: 0,
			output_tokens: 0,
			cache_read_input_tokens: 0,
			cache_write_input_tokens: 0,
			by_model: [],
			by_client: [],
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(
				"No AI Gateway spend for this user in the selected period.",
			),
		).toBeVisible();
	},
};

export const DrillInSummaryError: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryError: new Error("Failed to load spend"),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Failed to load spend")).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		expect(args.onSummaryRetry).toHaveBeenCalled();
	},
};
