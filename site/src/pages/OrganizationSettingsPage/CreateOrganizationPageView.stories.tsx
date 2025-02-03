import type { Meta, StoryObj } from "@storybook/react";
import { mockApiError } from "testHelpers/entities";
import { CreateOrganizationPageView } from "./CreateOrganizationPageView";

const meta: Meta<typeof CreateOrganizationPageView> = {
	title: "pages/CreateOrganizationPageView",
	component: CreateOrganizationPageView,
	args: {
		isEntitled: true,
	},
};

export default meta;
type Story = StoryObj<typeof CreateOrganizationPageView>;

export const Example: Story = {};

export const NotEntitled: Story = {
	args: {
		isEntitled: false,
	},
};

export const WithError: Story = {
	args: { error: "Oh no!" },
};

export const InvalidName: Story = {
	args: {
		error: mockApiError({
			message: "Display name is bad",
			validations: [
				{
					field: "display_name",
					detail: "That display name is terrible. What were you thinking?",
				},
			],
		}),
	},
};
