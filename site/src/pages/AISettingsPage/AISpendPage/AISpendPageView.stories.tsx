import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent, within } from "storybook/test";
import {
	MockOrganization,
	MockOrganization2,
	mockApiQueryError,
} from "#/testHelpers/entities";
import { AISpendPageView } from "./AISpendPageView";

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
		isOrganizationsLoading: false,
		organizationsError: null,
		days: { startDate: "2026-03-06", endDate: "2026-03-12" },
		onDaysChange: fn(),
		overviewError: null,
	},
} satisfies Meta<typeof AISpendPageView>;

export default meta;
type Story = StoryObj<typeof AISpendPageView>;

export const Overview: Story = {};

export const Mobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
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

export const OverviewError: Story = {
	args: {
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
