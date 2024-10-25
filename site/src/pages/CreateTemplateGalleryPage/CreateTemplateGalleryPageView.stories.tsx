import type { Meta, StoryObj } from "@storybook/react";
import { chromatic } from "testHelpers/chromatic";
import {
	MockTemplateExample,
	MockTemplateExample2,
	mockApiError,
} from "testHelpers/entities";
import { getTemplatesByTag } from "utils/starterTemplates";
import { CreateTemplateGalleryPageView } from "./CreateTemplateGalleryPageView";

const meta: Meta<typeof CreateTemplateGalleryPageView> = {
	title: "pages/CreateTemplateGalleryPage",
	parameters: { chromatic },
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
