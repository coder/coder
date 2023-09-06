import { Meta, StoryObj } from "@storybook/react";
import {
  MockTemplate,
  MockTemplateVersion,
  MockTemplateVersion3,
  MockWorkspaceResource,
  MockWorkspaceResource2,
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
    resources: [MockWorkspaceResource, MockWorkspaceResource2],
  },
};

export const NoIcon: Story = {
  args: {
    template: { ...MockTemplate, icon: "" },
    activeVersion: MockTemplateVersion,
    resources: [MockWorkspaceResource, MockWorkspaceResource2],
  },
};

export const SmallViewport: Story = {
  args: {
    template: MockTemplate,
    activeVersion: {
      ...MockTemplateVersion,
      readme: `---
    name:Template test
    ---
    ## Instructions
    You can add instructions here

    [Some link info](https://coder.com)
    \`\`\`
    # This is a really long sentence to test that the code block wraps into a new line properly.
    \`\`\`
    `,
    },
    resources: [MockWorkspaceResource, MockWorkspaceResource2],
  },
};

SmallViewport.parameters = {
  chromatic: { viewports: [600] },
};

export const WithDeprecatedParameters: Story = {
  args: {
    template: MockTemplate,
    activeVersion: MockTemplateVersion3,
    resources: [MockWorkspaceResource, MockWorkspaceResource2],
  },
};
