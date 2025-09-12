import { chromatic } from "testHelpers/chromatic";
import { MockTemplateExample, mockApiError } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { StarterTemplatePageView } from "./StarterTemplatePageView";

const meta: Meta<typeof StarterTemplatePageView> = {
	title: "pages/StarterTemplatePage",
	parameters: { chromatic },
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
