import type { Meta, StoryObj } from "@storybook/react-vite";
import type * as TypesGen from "#/api/typesGenerated";
import { mockSuccessResult } from "#/components/PaginationWidget/PaginationContainer.mocks";
import type { UsePaginatedQueryResult } from "#/hooks/usePaginatedQuery";
import {
	CoderAgentsUsagePageView,
	DEFAULT_RANGE,
} from "./CoderAgentsUsagePageView";

const mockUsers: TypesGen.AgentRuntimeInsightsUser[] = [
	{
		user_id: "1",
		username: "alice",
		avatar_url: "",
		total_ms: 12 * 60 * 60 * 1000,
		message_count: 340,
	},
	{
		user_id: "2",
		username: "bob",
		avatar_url: "",
		total_ms: 6 * 60 * 60 * 1000,
		message_count: 120,
	},
	{
		user_id: "3",
		username: "carol",
		avatar_url: "",
		total_ms: 2 * 60 * 60 * 1000,
		message_count: 42,
	},
];

const mockUsersQuery = {
	...mockSuccessResult,
	data: { users: mockUsers, count: mockUsers.length },
} as unknown as UsePaginatedQueryResult<TypesGen.AgentRuntimeInsightsByUserResponse>;

const mockSummary: TypesGen.AgentRuntimeInsightsResponse = {
	start_time: DEFAULT_RANGE.startTime,
	end_time: DEFAULT_RANGE.endTime,
	total_ms: 20 * 60 * 60 * 1000,
	by_day: [
		{ day: "2024-01-01", total_ms: 2 * 60 * 60 * 1000 },
		{ day: "2024-01-02", total_ms: 5 * 60 * 60 * 1000 },
		{ day: "2024-01-03", total_ms: 1 * 60 * 60 * 1000 },
		{ day: "2024-01-04", total_ms: 8 * 60 * 60 * 1000 },
		{ day: "2024-01-05", total_ms: 4 * 60 * 60 * 1000 },
	],
};

const meta: Meta<typeof CoderAgentsUsagePageView> = {
	title: "pages/AISettingsPage/CoderAgentsUsagePageView",
	component: CoderAgentsUsagePageView,
	args: {
		range: DEFAULT_RANGE,
		onRangeChange: () => {},
		summaryData: mockSummary,
		summaryError: undefined,
		isLoadingSummary: false,
		usersQuery: mockUsersQuery,
		sortColumn: "totalMs",
		sortDirection: "desc",
		onSortChange: () => {},
	},
};

export default meta;
type Story = StoryObj<typeof CoderAgentsUsagePageView>;

export const Loaded: Story = {};

export const Loading: Story = {
	args: {
		isLoadingSummary: true,
		summaryData: undefined,
		usersQuery: {
			...mockUsersQuery,
			isLoading: true,
			data: undefined,
		} as unknown as UsePaginatedQueryResult<TypesGen.AgentRuntimeInsightsByUserResponse>,
	},
};

export const Empty: Story = {
	args: {
		summaryData: { ...mockSummary, total_ms: 0, by_day: [] },
		usersQuery: {
			...mockUsersQuery,
			data: { users: [], count: 0 },
		} as unknown as UsePaginatedQueryResult<TypesGen.AgentRuntimeInsightsByUserResponse>,
	},
};
