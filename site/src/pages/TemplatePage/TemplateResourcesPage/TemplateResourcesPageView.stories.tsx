import type { Meta, StoryObj } from "@storybook/react";
import {
	MockTemplate,
	MockWorkspaceResource,
	MockWorkspaceVolumeResource,
} from "testHelpers/entities";
import { TemplateResourcesPageView } from "./TemplateResourcesPageView";

const meta: Meta<typeof TemplateResourcesPageView> = {
	title: "pages/TemplatePage/TemplateResourcesPageView",
	component: TemplateResourcesPageView,
};

export default meta;
type Story = StoryObj<typeof TemplateResourcesPageView>;

export const Example: Story = {
	args: {
		template: MockTemplate,
		resources: [MockWorkspaceResource, MockWorkspaceVolumeResource],
	},
};

export const NoIcon: Story = {
	args: {
		template: { ...MockTemplate, icon: "" },
		resources: [MockWorkspaceResource, MockWorkspaceVolumeResource],
	},
};
