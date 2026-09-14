import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockMenu } from "#/components/Filter/storyHelpers";
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
		dimensions: {},
		filterMenus: { provider: MockMenu, client: MockMenu, model: MockMenu },
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
};

export const NotEnabled: Story = {
	args: {
		isEnabled: false,
	},
};

export const Loading: Story = {
	args: {
		usersQuery: mockUsersQuery({ isLoading: true }),
	},
};

export const Empty: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: { ...mockUsersResponse, count: 0, users: [] },
		}),
	},
};

export const Users: Story = {};

export const SortUsers: Story = {
	play: async ({ canvasElement }) => {
		const table = within(
			within(canvasElement).getByRole("table", { name: "Spend by user" }),
		);
		await userEvent.click(table.getByRole("button", { name: "Requests" }));
	},
};

export const DeploymentOverview: Story = {
	args: {
		summaryData: MockAIGatewaySpendUserSummary,
		usersQuery: mockUsersQuery({
			data: { ...mockUsersResponse, count: 1, users: [MockAIGatewaySpendUser] },
		}),
	},
};

export const ProviderPagination: Story = {
	args: {
		summaryData: {
			...MockAIGatewaySpendUserSummary,
			provider_count: 101,
			by_provider: Array.from({ length: 11 }, (_, i) => ({
				...MockAIGatewaySpendUserSummary.by_provider[0],
				provider: "anthropic",
				provider_name: `provider-${i + 1}`,
			})),
		},
	},
	play: async ({ canvasElement }) => {
		const providers = within(
			within(canvasElement).getByRole("region", { name: "Spend by provider" }),
		);
		await userEvent.click(providers.getByRole("button", { name: "Next page" }));
	},
};

export const DeploymentSummaryError: Story = {
	args: { summaryError: new Error("Unable to load deployment spend") },
};

export const DeploymentRefetchError: Story = {
	args: {
		summaryData: MockAIGatewaySpendUserSummary,
		summaryError: new Error("Spend refresh failed"),
	},
};

export const DeploymentMobile: Story = {
	...DeploymentOverview,
	globals: { viewport: { value: "mobile2", isRotated: false } },
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
};

export const UsersClampedToRetention: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: { ...mockUsersResponse, start_date: "2026-02-26T12:00:00Z" },
		}),
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
};

export const UsersSearchNoMatch: Story = {
	args: {
		searchFilter: "nobody",
		usersQuery: mockUsersQuery({
			data: { ...mockUsersResponse, count: 0, users: [] },
		}),
	},
};

export const Refreshing: Story = {
	args: {
		usersQuery: mockUsersQuery({ data: mockUsersResponse, isFetching: true }),
	},
};

export const UsersError: Story = {
	args: {
		usersQuery: mockUsersQuery({
			error: new Error("Failed to load spend data"),
		}),
	},
};

export const UsersRefetchError: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: mockUsersResponse,
			error: new Error("Failed to refresh spend data"),
		}),
	},
};

export const DrillInLoading: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		isDrillInUserLoading: true,
	},
};

export const DrillInUserError: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUserError: new Error("User not found"),
	},
};

export const DrillInSummaryLoading: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		isSummaryLoading: true,
	},
};

export const DrillIn: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryData: MockAIGatewaySpendUserSummary,
	},
};

export const DrillInFiltered: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryData: MockAIGatewaySpendUserSummary,
		dimensions: { provider_name: "anthropic-main", client: "Claude Code" },
		filterMenus: {
			provider: {
				...MockMenu,
				selectedOption: { label: "Anthropic", value: "anthropic-main" },
			},
			client: {
				...MockMenu,
				selectedOption: { label: "Claude Code", value: "Claude Code" },
			},
			model: MockMenu,
		},
	},
};

export const DrillInWithUnpricedRequests: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryData: {
			...MockAIGatewaySpendUserSummary,
			unpriced_request_count: 1,
			by_model: MockAIGatewaySpendUserSummary.by_model.map((model, i) =>
				i === 1 ? { ...model, unpriced_request_count: 1 } : model,
			),
		},
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
};

export const DrillInOutsideRetention: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryData: {
			...MockAIGatewaySpendUserSummary,
			start_date: "2026-03-12T00:00:00Z",
			total_cost_micros: 0,
			request_count: 0,
			session_count: 0,
			model_count: 0,
			client_count: 0,
			by_model: [],
			by_client: [],
		},
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
};

export const DrillInSummaryError: Story = {
	args: {
		drillInUserId: MockAIGatewaySpendUser.id,
		drillInUser: mockUserProfile,
		summaryError: new Error("Failed to load spend"),
	},
};
