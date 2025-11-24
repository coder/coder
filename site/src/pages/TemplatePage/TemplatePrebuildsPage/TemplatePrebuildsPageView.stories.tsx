import { MockTemplate } from "testHelpers/entities";
import { withGlobalSnackbar } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { spyOn, userEvent, within } from "storybook/test";
import { TemplatePrebuildsPageView } from "./TemplatePrebuildsPage";

const meta: Meta<typeof TemplatePrebuildsPageView> = {
	title: "pages/TemplatePage/TemplatePrebuildsPageView",
	component: TemplatePrebuildsPageView,
	args: {
		templateId: MockTemplate.id,
	},
	decorators: [withGlobalSnackbar],
};

export default meta;
type Story = StoryObj<typeof TemplatePrebuildsPageView>;

export const NoPresetsInvalidated: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "invalidateTemplatePresets").mockResolvedValue({
			invalidated: [],
		});
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const groupLabel = await canvas.findByText("Invalidate now");
		await user.click(groupLabel);
	},
};

export const PresetsInvalidated: Story = {
	play: async ({ canvasElement }) => {
		spyOn(API, "invalidateTemplatePresets").mockResolvedValue({
			invalidated: [
				{
					preset_name: "First preset",
					template_name: "Super template",
					template_version_name: "abcdef",
				},
				{
					preset_name: "Second preset",
					template_name: "Super template",
					template_version_name: "abcdef",
				},
			],
		});
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const groupLabel = await canvas.findByText("Invalidate now");
		await user.click(groupLabel);
	},
};
