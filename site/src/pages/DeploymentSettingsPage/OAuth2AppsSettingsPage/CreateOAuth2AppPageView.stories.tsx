import { mockApiError } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { CreateOAuth2AppPageView } from "./CreateOAuth2AppPageView";

const meta: Meta = {
	title: "pages/DeploymentSettingsPage/CreateOAuth2AppPageView",
	component: CreateOAuth2AppPageView,
};
export default meta;

type Story = StoryObj<typeof CreateOAuth2AppPageView>;

export const Updating: Story = {
	args: {
		isUpdating: true,
	},
};

export const WithError: Story = {
	args: {
		error: mockApiError({
			message: "Validation failed",
			validations: [
				{
					field: "name",
					detail: "name error",
				},
				{
					field: "callback_url",
					detail: "url error",
				},
				{
					field: "icon",
					detail: "icon error",
				},
			],
		}),
	},
};

export const Default: Story = {};
