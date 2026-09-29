import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import type { OrganizationAISpendReport } from "#/api/typesGenerated";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockOrganizationAISpendReport,
	MockOrganizationAISpendUser,
} from "#/testHelpers/entities";
import { type SpendReportQuery, SpendUsersTable } from "./SpendUsersTable";

const loadedReportQuery = (
	report: OrganizationAISpendReport,
): SpendReportQuery => ({
	...mockSuccessResult,
	totalRecords: report.count,
	currentOffsetStart: report.count === 0 ? 0 : 1,
	data: report,
	isLoading: false,
	isFetching: false,
	error: null,
	refetch: fn(),
});

const pendingReportQuery = (): SpendReportQuery => ({
	...mockInitialRenderResult,
	data: undefined,
	isLoading: true,
	isFetching: true,
	error: null,
	refetch: fn(),
});

const mockUnpricedUsageReport: OrganizationAISpendReport = {
	...MockOrganizationAISpendReport,
	totals: { cost_micros: 3_500_000, unpriced_usage_count: 4 },
	users: MockOrganizationAISpendReport.users.map((user, index) => ({
		...user,
		unpriced_usage_count: index === 0 ? 3 : 1,
	})),
};

// Every row collapses into count badges, so the badges are told apart only
// by their row header.
const mockMultipleDimensionsReport: OrganizationAISpendReport = {
	...MockOrganizationAISpendReport,
	users: MockOrganizationAISpendReport.users.map((user) => ({
		...user,
		providers: MockOrganizationAISpendUser.providers,
		models: MockOrganizationAISpendUser.models,
		clients: MockOrganizationAISpendUser.clients,
	})),
};

const meta = {
	title: "pages/AISettingsPage/SpendPage/SpendUsersTable",
	component: SpendUsersTable,
} satisfies Meta<typeof SpendUsersTable>;

export default meta;
type Story = StoryObj<typeof SpendUsersTable>;

export const Loading: Story = {
	args: { reportQuery: pendingReportQuery() },
};

export const Empty: Story = {
	args: {
		reportQuery: loadedReportQuery({
			...MockOrganizationAISpendReport,
			count: 0,
			totals: { cost_micros: 0, unpriced_usage_count: 0 },
			users: [],
		}),
	},
};

export const LoadError: Story = {
	args: {
		reportQuery: {
			...pendingReportQuery(),
			isLoading: false,
			isFetching: false,
			error: new Error("Unable to load organization spend"),
		},
	},
};

export const RefetchError: Story = {
	args: {
		reportQuery: {
			...loadedReportQuery(MockOrganizationAISpendReport),
			error: new Error("Spend refresh failed"),
		},
	},
};

export const Refreshing: Story = {
	args: {
		reportQuery: {
			...loadedReportQuery(MockOrganizationAISpendReport),
			isFetching: true,
		},
	},
};

export const Users: Story = {
	args: { reportQuery: loadedReportQuery(MockOrganizationAISpendReport) },
};

export const UnpricedUsage: Story = {
	args: { reportQuery: loadedReportQuery(mockUnpricedUsageReport) },
};

export const CostSetupTooltip: Story = {
	args: { reportQuery: loadedReportQuery(mockUnpricedUsageReport) },
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", {
				name: `Cost setup for ${MockOrganizationAISpendUser.name}`,
			}),
		);
		await screen.findByRole("tooltip");
	},
};

export const TotalCostSetupKeyboard: Story = {
	args: { reportQuery: loadedReportQuery(mockUnpricedUsageReport) },
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getByRole("button", { name: "Cost setup for total spend" })
			.focus();
		await screen.findByRole("tooltip");
	},
};

export const MultipleDimensions: Story = {
	args: { reportQuery: loadedReportQuery(mockMultipleDimensionsReport) },
};

export const ClientsList: Story = {
	args: { reportQuery: loadedReportQuery(mockMultipleDimensionsReport) },
	play: async ({ canvasElement }) => {
		within(within(canvasElement).getByRole("row", { name: /alice/ }))
			.getByRole("button", { name: "2 clients" })
			.focus();
		await screen.findByRole("tooltip");
	},
};

export const ModelsList: Story = {
	args: { reportQuery: loadedReportQuery(mockMultipleDimensionsReport) },
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(
				within(canvasElement).getByRole("row", { name: /alice/ }),
			).getByRole("button", { name: "2 models" }),
		);
		await screen.findByRole("tooltip");
	},
};

// A list taller than the viewport scrolls inside the tooltip.
export const LongModelsList: Story = {
	args: {
		reportQuery: loadedReportQuery({
			...mockMultipleDimensionsReport,
			users: mockMultipleDimensionsReport.users.map((user) => ({
				...user,
				models: Array.from({ length: 40 }, (_, i) =>
					i % 2 === 0 ? `claude-model-${i}` : `gpt-model-${i}`,
				),
			})),
		}),
	},
	globals: { viewport: { value: "mobile2", isRotated: false } },
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(
				within(canvasElement).getByRole("row", { name: /alice/ }),
			).getByRole("button", { name: "40 models" }),
		);
		await screen.findByRole("tooltip");
	},
};
