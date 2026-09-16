import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockTemplateExample, mockApiError } from "#/testHelpers/entities";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

const meta: Meta<typeof StarterTemplatePageView> = {
	title: "pages/StarterTemplatePage",
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
