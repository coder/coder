import { Story } from "@storybook/react"
import * as Mocks from "testHelpers/renderHelpers"
import {
  TemplateSummaryPageView,
  TemplateSummaryPageViewProps,
} from "./TemplateSummaryPageView"

export default {
  title: "pages/TemplateSummaryPageView",
  component: TemplateSummaryPageView,
}

const Template: Story<TemplateSummaryPageViewProps> = (args) => (
  <TemplateSummaryPageView {...args} />
)

export const Example = Template.bind({})
Example.args = {
  template: Mocks.MockTemplate,
  activeTemplateVersion: Mocks.MockTemplateVersion,
  templateResources: [
    Mocks.MockWorkspaceResource,
    Mocks.MockWorkspaceResource2,
  ],
  templateVersions: [Mocks.MockTemplateVersion],
}

export const NoIcon = Template.bind({})
NoIcon.args = {
  template: { ...Mocks.MockTemplate, icon: "" },
  activeTemplateVersion: Mocks.MockTemplateVersion,
  templateResources: [
    Mocks.MockWorkspaceResource,
    Mocks.MockWorkspaceResource2,
  ],
  templateVersions: [Mocks.MockTemplateVersion],
}

export const SmallViewport = Template.bind({})
SmallViewport.args = {
  template: Mocks.MockTemplate,
  activeTemplateVersion: {
    ...Mocks.MockTemplateVersion,
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
  templateResources: [
    Mocks.MockWorkspaceResource,
    Mocks.MockWorkspaceResource2,
  ],
  templateVersions: [Mocks.MockTemplateVersion],
}
SmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
