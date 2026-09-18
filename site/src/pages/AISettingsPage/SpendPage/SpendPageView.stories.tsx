import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, userEvent, within } from "storybook/test";
import { MockMenu } from "#/components/Filter/storyHelpers";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendReport,
	MockOrganizationAISpendUser,
} from "#/testHelpers/entities";
import type { SpendReportQuery } from "./components/SpendUsersTable";
import { SpendPageView } from "./SpendPageView";

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
	title: "pages/AISettingsPage/SpendPage/SpendPageView",
	component: SpendPageView,
	args: {
		isEntitled: true,
		isEnabled: true,
		now: new Date("2026-03-12T12:00:00Z"),
		organizations: [MockOrganization, MockOrganization2],
		organization: MockOrganization,
		onOrganizationChange: fn(),
		isOrganizationsLoading: false,
		organizationsError: null,
		dateRange: {
			startDate: new Date("2026-02-10T00:00:00Z"),
			endDate: new Date("2026-03-12T00:00:00Z"),
		},
		minDate: new Date("2026-01-12T00:00:00Z"),
		onDateRangeChange: fn(),
		filterMenus: { provider: MockMenu, client: MockMenu, model: MockMenu },
		reportQuery: mockReportQuery,
	},
} satisfies Meta<typeof SpendPageView>;

export default meta;
type Story = StoryObj<typeof SpendPageView>;

export const Paywall: Story = {
	args: { isEntitled: false },
};

export const NotEnabled: Story = {
	args: { isEnabled: false },
};

export const OrganizationsLoading: Story = {
	args: { isOrganizationsLoading: true },
};

export const OrganizationsError: Story = {
	args: {
		organizations: [],
		organization: undefined,
		organizationsError: new Error("Unable to load organizations"),
	},
};

export const OrganizationsRefetchError: Story = {
	args: { organizationsError: new Error("Unable to refresh organizations") },
};

export const NoPermittedOrganizations: Story = {
	args: { organizations: [], organization: undefined },
};

export const Loading: Story = {
	args: {
		dateRange: undefined,
		reportQuery: mockPendingReportQuery,
	},
};

export const LoadingExplicitRange: Story = {
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
				totals: { cost_micros: 3_500_000, unpriced_usage_count: 3 },
				users: [
					{ ...MockOrganizationAISpendUser, unpriced_usage_count: 3 },
					...MockOrganizationAISpendReport.users.slice(1),
				],
			},
		},
	},
};

export const UnpricedModelsTooltip: Story = {
	...UnpricedUsage,
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: "Unpriced models" }),
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

export const WithoutDimensionFilters: Story = {
	args: { filterMenus: undefined },
};

export const SingleOrganization: Story = {
	args: { organizations: [MockOrganization] },
};

export const OrganizationMenu: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", {
				name: `Organization ${MockOrganization.display_name}`,
			}),
		);
	},
};

export const RequestedOrganizationDenied: Story = {
	args: { organization: undefined },
};

export const Mobile: Story = {
	globals: { viewport: { value: "mobile2", isRotated: false } },
};

// The content width a 1024px viewport leaves beside the settings sidebar.
export const NarrowContainer: Story = {
	decorators: [
		(Story) => (
			<div className="w-[580px]">
				<Story />
			</div>
		),
	],
};
