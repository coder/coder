import type { Meta, StoryObj } from "@storybook/react-vite";
import { deploymentConfigQueryKey } from "#/api/queries/deployment";
import { chromatic } from "#/testHelpers/chromatic";
import {
	MockDeploymentConfig,
	MockTemplateExample,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

const meta: Meta<typeof StarterTemplatePageView> = {
	title: "pages/StarterTemplatePage",
	parameters: {
		chromatic,
		user: MockUserOwner,
		queries: [
			{
				key: deploymentConfigQueryKey,
				data: MockDeploymentConfig,
			},
		],
	},
	decorators: [withAuthProvider],
	component: StarterTemplatePageView,
};

export default meta;
type Story = StoryObj<typeof StarterTemplatePageView>;

export const Example: Story = {
	args: {
		error: undefined,
		starterTemplate: MockTemplateExample,
	},
};
export const WithError: Story = {
	args: {
		error: mockApiError({
			message: `Example ${MockTemplateExample.id} not found.`,
		}),
		starterTemplate: undefined,
	},
};
