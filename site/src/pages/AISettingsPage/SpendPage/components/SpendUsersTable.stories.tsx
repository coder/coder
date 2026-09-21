import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockOrganizationAISpendReport,
	MockOrganizationAISpendUser,
} from "#/testHelpers/entities";
import { type SpendReportQuery, SpendUsersTable } from "./SpendUsersTable";

const mockReportQuery = {
	...mockSuccessResult,
	totalRecords: MockOrganizationAISpendReport.count,
	data: MockOrganizationAISpendReport,
	isLoading: false,
	isFetching: false,
	error: null,
	refetch: fn(),
} satisfies SpendReportQuery;

const mockPendingReportQuery = {
	...mockInitialRenderResult,
	data: undefined,
	isLoading: true,
	isFetching: true,
	error: null,
	refetch: fn(),
} satisfies SpendReportQuery;

const meta = {
	title: "pages/AISettingsPage/SpendPage/SpendUsersTable",
	component: SpendUsersTable,
	args: { reportQuery: mockReportQuery },
} satisfies Meta<typeof SpendUsersTable>;

export default meta;
type Story = StoryObj<typeof SpendUsersTable>;

export const Loading: Story = {
	args: { reportQuery: mockPendingReportQuery },
};

export const Empty: Story = {
	args: {
		reportQuery: {
			...mockReportQuery,
			totalRecords: 0,
			currentOffsetStart: 0,
			data: {
				...MockOrganizationAISpendReport,
				count: 0,
				totals: { cost_micros: 0, unpriced_usage_count: 0 },
				users: [],
			},
		},
	},
};

export const LoadError: Story = {
	args: {
		reportQuery: {
			...mockPendingReportQuery,
			isLoading: false,
			isFetching: false,
			error: new Error("Unable to load organization spend"),
		},
	},
};

export const RefetchError: Story = {
	args: {
		reportQuery: {
			...mockReportQuery,
			error: new Error("Spend refresh failed"),
		},
	},
};

export const Refreshing: Story = {
	args: {
		reportQuery: { ...mockReportQuery, isFetching: true },
	},
};

export const Users: Story = {};

export const UnpricedUsage: Story = {
	args: {
		reportQuery: {
			...mockReportQuery,
			data: {
				...MockOrganizationAISpendReport,
				totals: { cost_micros: 3_500_000, unpriced_usage_count: 4 },
				users: MockOrganizationAISpendReport.users.map((user, index) => ({
					...user,
					unpriced_usage_count: index === 0 ? 3 : 1,
				})),
			},
		},
	},
};

export const UnpricedModelsTooltip: Story = {
	...UnpricedUsage,
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", {
				name: `Unpriced models for ${MockOrganizationAISpendUser.name}`,
			}),
		);
		await screen.findByRole("tooltip");
	},
};

export const TotalUnpricedModelsKeyboard: Story = {
	...UnpricedUsage,
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: "Unpriced models in total spend" })
			.focus();
		await screen.findByRole("tooltip");
	},
};

export const ClientsList: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement).getByRole("button", { name: "2 clients" }).focus();
		await screen.findByRole("tooltip");
	},
};

export const ModelsList: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: "2 models" }),
		);
		await screen.findByRole("tooltip");
	},
};
