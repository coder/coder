import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	MockOrganizationAISpendOverview,
	MockOrganizationAISpendUser,
	mockApiQueryError,
} from "#/testHelpers/entities";
import { CostDetailsDialog } from "./CostDetailsDialog";

const meta = {
	title: "pages/AISettingsPage/AISpendPage/CostDetailsDialog",
	component: CostDetailsDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		onCloseAutoFocus: fn(),
		user: MockOrganizationAISpendOverview.users[0],
		details: MockOrganizationAISpendUser,
		detailsError: null,
		period: MockOrganizationAISpendOverview,
		organizationName: "Coder",
		onOpenDetailedReport: fn(),
	},
} satisfies Meta<typeof CostDetailsDialog>;

export default meta;
type Story = StoryObj<typeof CostDetailsDialog>;

export const Default: Story = {};

/** The users response can miss a listed user when the spend changed between the requests. */
export const CostDetailsUnavailable: Story = {
	args: { details: null },
};

/** A model can be partly priced, or have no price at all. */
export const UnpricedUsage: Story = {
	args: {
		details: {
			...MockOrganizationAISpendUser,
			provider_spend: [
				{
					...MockOrganizationAISpendUser.provider_spend[0],
					has_unpriced_usage: true,
				},
				MockOrganizationAISpendUser.provider_spend[1],
			],
			model_spend: [
				{
					...MockOrganizationAISpendUser.model_spend[0],
					has_unpriced_usage: true,
				},
				MockOrganizationAISpendUser.model_spend[1],
				{ name: "llama-4-maverick", cost_micros: 0, has_unpriced_usage: true },
			],
		},
	},
};

/** The breakdown scrolls when it is taller than the dialog allows. */
export const ManyModels: Story = {
	args: {
		user: {
			...MockOrganizationAISpendOverview.users[0],
			spend_micros: 210_000_000,
		},
		details: {
			...MockOrganizationAISpendUser,
			model_spend: Array.from({ length: 20 }, (_, i) => ({
				name: `claude-sonnet-${i + 1}`,
				cost_micros: (20 - i) * 1_000_000,
				has_unpriced_usage: false,
			})),
		},
	},
};

export const Loading: Story = {
	args: { details: undefined },
};

export const CostDetailsError: Story = {
	args: {
		details: undefined,
		detailsError: mockApiQueryError({
			message: "Failed to load the cost details.",
		}),
	},
};
