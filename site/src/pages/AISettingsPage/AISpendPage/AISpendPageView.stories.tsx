import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import {
	MockOrganization,
	MockOrganization2,
	MockOrganizationAISpendOverview,
	MockOrganizationAISpendReport,
	MockUnpricedAIModels,
	mockApiQueryError,
} from "#/testHelpers/entities";
import { AISpendPageView } from "./AISpendPageView";

// The first user used both unpriced models, so their row warns like the total.
const [mockSpender, ...mockOtherSpenders] = MockOrganizationAISpendReport.users;
const mockTopSpenderDetails = [
	{
		...mockSpender,
		provider_spend: mockSpender.provider_spend.map((provider) => ({
			...provider,
			has_unpriced_usage: true,
		})),
		model_spend: [
			...mockSpender.model_spend,
			...MockUnpricedAIModels.models.map((model) => ({
				name: model.model,
				cost_micros: 0,
				has_unpriced_usage: true,
			})),
		],
	},
	...mockOtherSpenders,
];

const meta = {
	title: "pages/AISettingsPage/AISpendPage/AISpendPageView",
	component: AISpendPageView,
	args: {
		isEntitled: true,
		isEnabled: true,
		now: new Date("2026-03-12T12:00:00Z"),
		organizations: [MockOrganization, MockOrganization2],
		organization: MockOrganization,
		onOrganizationChange: fn(),
		tab: "overview",
		onTabChange: fn(),
		isOrganizationsLoading: false,
		organizationsError: null,
		days: { startDate: "2026-03-06", endDate: "2026-03-12" },
		onDaysChange: fn(),
		onOpenDetailedReport: fn(),
		overview: MockOrganizationAISpendOverview,
		overviewError: null,
		unpricedModels: MockUnpricedAIModels.models,
		isUnpricedModelsLoading: false,
		topSpenderDetails: mockTopSpenderDetails,
		topSpenderDetailsError: null,
		canViewSessions: true,
	},
} satisfies Meta<typeof AISpendPageView>;

export default meta;
type Story = StoryObj<typeof AISpendPageView>;

export const Overview: Story = {};

export const DetailedReport: Story = {
	args: { tab: "detailed-report" },
};

export const OverviewMobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
};

export const DetailedReportMobile: Story = {
	args: { tab: "detailed-report" },
	globals: { viewport: { value: "iphone12", isRotated: false } },
};

export const OverviewLoading: Story = {
	args: {
		overview: undefined,
		unpricedModels: undefined,
		isUnpricedModelsLoading: true,
		topSpenderDetails: undefined,
	},
};

export const NoSpend: Story = {
	args: {
		overview: {
			...MockOrganizationAISpendOverview,
			count: 0,
			totals: { spend_micros: 0, median_spend_micros: 0 },
			intervals: MockOrganizationAISpendOverview.intervals.map((interval) => ({
				...interval,
				spend_micros: 0,
			})),
			users: [],
		},
		unpricedModels: [],
	},
};

export const SingleOrganization: Story = {
	args: { organizations: [MockOrganization] },
};

export const DatePickerOpen: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: /Mar 6, 2026/ }));
	},
};

/** A failed refetch keeps the cached spend on screen. */
export const OverviewRefetchError: Story = {
	args: {
		overviewError: mockApiQueryError({
			message: "Unable to refresh AI spend.",
		}),
	},
};

export const OverviewError: Story = {
	args: {
		overview: undefined,
		overviewError: mockApiQueryError({ message: "Unable to load AI spend." }),
	},
};

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
		organizationsError: mockApiQueryError({
			message: "Unable to load organizations.",
		}),
	},
};

export const OrganizationsRefetchError: Story = {
	args: {
		organizationsError: mockApiQueryError({
			message: "Unable to refresh organizations.",
		}),
	},
};

export const NoPermittedOrganizations: Story = {
	args: { organizations: [], organization: undefined },
};

export const OrganizationUnavailable: Story = {
	args: { organization: undefined },
};
