import type { Meta, StoryObj } from "@storybook/react-vite";
import { deploymentConfigQueryKey } from "#/api/queries/deployment";
import { chromatic } from "#/testHelpers/chromatic";
import {
	MockDeploymentConfig,
	MockTemplateExample,
	MockTemplateExample2,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { getTemplatesByTag } from "#/utils/starterTemplates";
import { CreateTemplateGalleryPageView } from "./CreateTemplateGalleryPageView";

const meta: Meta<typeof CreateTemplateGalleryPageView> = {
	title: "pages/CreateTemplateGalleryPage",
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
	component: CreateTemplateGalleryPageView,
};

export default meta;
type Story = StoryObj<typeof CreateTemplateGalleryPageView>;

export const Example: Story = {
	args: {
		error: undefined,
		starterTemplatesByTag: getTemplatesByTag([
			MockTemplateExample,
			MockTemplateExample2,
		]),
	},
};

export const WithError: Story = {
	args: {
		error: mockApiError({
			message: "Error on loading the template examples",
		}),
		starterTemplatesByTag: undefined,
	},
};
