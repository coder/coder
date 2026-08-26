import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	MockTemplateExample,
	MockTemplateExample2,
	mockApiError,
} from "#/testHelpers/entities";
import { getTemplatesByTag } from "#/utils/starterTemplates";
import { StarterTemplatesPageView } from "./StarterTemplatesPageView";

const meta: Meta<typeof StarterTemplatesPageView> = {
	title: "pages/StarterTemplatesPage",
	component: StarterTemplatesPageView,
	args: {
		templateBuilderEnabled: true,
	},
};

export default meta;
type Story = StoryObj<typeof StarterTemplatesPageView>;

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
