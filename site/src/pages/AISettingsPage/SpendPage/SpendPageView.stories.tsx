import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import type * as TypesGen from "#/api/typesGenerated";
import { MockMenu } from "#/components/Filter/storyHelpers";
import { mockPaginationResultBase } from "#/components/PaginationWidget/PaginationContainer.mocks";
import {
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendReport,
	MockOrganizationAISpendUser,
} from "#/testHelpers/entities";
import { SpendPageView, type SpendUsersQuery } from "./SpendPageView";

const defaultDateRange = {
	startDate: new Date("2026-02-10T00:00:00Z"),
	endDate: new Date("2026-03-12T00:00:00Z"),
};

function mockUsersQuery(
	opts: {
		data?: TypesGen.OrganizationAISpendReport;
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
		now: new Date("2026-03-12T12:00:00Z"),
		organizations: [MockOrganization, MockOrganization2],
		organization: MockOrganization,
		onOrganizationChange: fn(),
		requestedOrganizationDenied: false,
		isOrganizationsLoading: false,
		organizationsError: null,
		dateRange: defaultDateRange,
		minDate: new Date("2026-01-12T00:00:00Z"),
		onDateRangeChange: fn(),
		filterMenus: { provider: MockMenu, client: MockMenu, model: MockMenu },
		usersQuery: mockUsersQuery({ data: MockOrganizationAISpendReport }),
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
		usersQuery: mockUsersQuery({ isLoading: true }),
	},
};

export const LoadingExplicitRange: Story = {
	args: { usersQuery: mockUsersQuery({ isLoading: true }) },
};

export const Empty: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: {
				...MockOrganizationAISpendReport,
				count: 0,
				totals: { cost_micros: 0, unpriced_usage_count: 0 },
				users: [],
			},
		}),
	},
};

export const LoadError: Story = {
	args: {
		usersQuery: mockUsersQuery({
			error: new Error("Unable to load organization spend"),
		}),
	},
};

export const RefetchError: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: MockOrganizationAISpendReport,
			error: new Error("Spend refresh failed"),
		}),
	},
};

export const Refreshing: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: MockOrganizationAISpendReport,
			isFetching: true,
		}),
	},
};

export const Users: Story = {};

export const UnpricedUsage: Story = {
	args: {
		usersQuery: mockUsersQuery({
			data: {
				...MockOrganizationAISpendReport,
				totals: { cost_micros: 3_500_000, unpriced_usage_count: 3 },
				users: [
					{ ...MockOrganizationAISpendUser, unpriced_usage_count: 3 },
					...MockOrganizationAISpendReport.users.slice(1),
				],
			},
		}),
	},
};

export const UnpricedModelsTooltip: Story = {
	...UnpricedUsage,
	play: async ({ canvasElement }) => {
		const table = within(
			within(canvasElement).getByRole("table", { name: "Spend by user" }),
		);
		await userEvent.hover(
			table.getByRole("button", { name: "Unpriced models" }),
		);
		await within(canvasElement.ownerDocument.body).findByRole("tooltip");
	},
};

export const TotalUnpricedModelsKeyboard: Story = {
	...UnpricedUsage,
	play: async ({ canvasElement }) => {
		within(canvasElement)
			.getAllByRole("button", { name: "Unpriced models" })[0]
			.focus();
		await within(canvasElement.ownerDocument.body).findByRole("tooltip");
	},
};

export const ClientsList: Story = {
	play: async ({ canvasElement }) => {
		within(canvasElement).getByRole("button", { name: "2 clients" }).focus();
		await within(canvasElement.ownerDocument.body).findByRole("tooltip");
	},
};

export const ModelsList: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.hover(
			within(canvasElement).getByRole("button", { name: "2 models" }),
		);
		await within(canvasElement.ownerDocument.body).findByRole("tooltip");
	},
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
	args: { requestedOrganizationDenied: true },
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
