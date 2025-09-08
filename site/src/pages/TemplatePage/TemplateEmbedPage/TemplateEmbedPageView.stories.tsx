import {
	MockTemplate,
	MockTemplateVersionParameter1,
	MockTemplateVersionParameter2,
	MockTemplateVersionParameter3,
	MockTemplateVersionParameter4,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { TemplateEmbedPageView } from "./TemplateEmbedPage";

const meta: Meta<typeof TemplateEmbedPageView> = {
	title: "pages/TemplatePage/TemplateEmbedPageView",
	component: TemplateEmbedPageView,
	args: {
		template: MockTemplate,
	},
};

export default meta;
type Story = StoryObj<typeof TemplateEmbedPageView>;

export const NoParameters: Story = {
	args: {
		templateParameters: [],
	},
};

export const WithParameters: Story = {
	args: {
		templateParameters: [
			MockTemplateVersionParameter1,
			MockTemplateVersionParameter2,
			MockTemplateVersionParameter3,
			MockTemplateVersionParameter4,
		],
	},
};

export const WrongWorkspaceName: Story = {
	args: {
		templateParameters: [MockTemplateVersionParameter1],
	},
	play: async () => {
		const workspaceName = await screen.findByRole("textbox", {
			name: "Workspace name",
		});
		await userEvent.type(workspaceName, "b@d");
	},
};
