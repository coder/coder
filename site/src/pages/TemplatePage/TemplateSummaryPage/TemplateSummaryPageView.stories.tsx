import { Meta, StoryObj } from "@storybook/react";
import {
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersion3,
  MockWorkspaceResource,
  MockWorkspaceVolumeResource,
} from "testHelpers/entities";
import { TemplateSummaryPageView } from "./TemplateSummaryPageView";

const meta: Meta<typeof TemplateSummaryPageView> = {
  title: "pages/TemplateSummaryPageView",
  component: TemplateSummaryPageView,
};

export default meta;
type Story = StoryObj<typeof TemplateSummaryPageView>;

export const Example: Story = {
  args: {
    template: MockTemplate,
    activeVersion: MockTemplateVersion,
    resources: [MockWorkspaceResource, MockWorkspaceVolumeResource],
  },
};

export const NoIcon: Story = {
  args: {
    template: { ...MockTemplate, icon: "" },
    activeVersion: MockTemplateVersion,
    resources: [MockWorkspaceResource, MockWorkspaceVolumeResource],
  },
};

export const WithDeprecatedParameters: Story = {
  args: {
    template: MockTemplate,
    activeVersion: MockTemplateVersion3,
    resources: [MockWorkspaceResource, MockWorkspaceVolumeResource],
  },
};
