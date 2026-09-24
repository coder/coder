import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import {
	mockInitialRenderResult,
	mockSuccessResult,
} from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendReport,
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
		period: {
			start: new Date("2026-02-10T00:00:00Z"),
			end: new Date("2026-03-12T00:00:00Z"),
		},
		minDate: new Date("2026-01-12T00:00:00Z"),
		onPeriodChange: fn(),
		filterQuery: "",
		onFilterQueryChange: fn(),
		canFilterDimensions: true,
		onExportCSV: fn(),
		isExportingCSV: false,
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
		period: {
			start: new Date("2026-03-05T12:00:00Z"),
			end: new Date("2026-03-12T12:00:00Z"),
			preset: "last_7d",
		},
		minDate: undefined,
		reportQuery: mockPendingReportQuery,
	},
};

export const Users: Story = {};

export const WithoutDimensionFilters: Story = {
	args: { canFilterDimensions: false },
};

export const PricingPreset: Story = {
	args: { filterQuery: "pricing:unconfigured" },
};

export const ManyFilterChips: Story = {
	args: {
		filterQuery:
			"user:alice group:engineering provider:openai model:gpt-4o client:cursor pricing:unconfigured",
	},
};

const openFilterMenu: Story["play"] = async ({ canvasElement }) => {
	await userEvent.click(
		within(canvasElement).getByRole("combobox", {
			name: "Search and filter users…",
		}),
	);
};

export const FilterMenu: Story = {
	play: openFilterMenu,
};

export const FilterMenuPricingSelected: Story = {
	args: { filterQuery: "pricing:unconfigured" },
	play: openFilterMenu,
};

export const OrganizationFlyout: Story = {
	play: async (context) => {
		await openFilterMenu(context);
		await userEvent.hover(
			await within(context.canvasElement).findByRole("option", {
				name: "Organization",
			}),
		);
	},
};

export const ExportingCSV: Story = {
	args: { isExportingCSV: true },
};

export const SingleOrganization: Story = {
	args: { organizations: [MockOrganization] },
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
