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
  activeVersion: Mocks.MockTemplateVersion,
  data: {
    resources: [Mocks.MockWorkspaceResource, Mocks.MockWorkspaceResource2],
    versions: [Mocks.MockTemplateVersion],
    daus: Mocks.MockTemplateDAUResponse,
  },
}

export const NoIcon = Template.bind({})
NoIcon.args = {
  template: { ...Mocks.MockTemplate, icon: "" },
  activeVersion: Mocks.MockTemplateVersion,
  data: {
    resources: [Mocks.MockWorkspaceResource, Mocks.MockWorkspaceResource2],
    versions: [Mocks.MockTemplateVersion],
    daus: Mocks.MockTemplateDAUResponse,
  },
}

export const SmallViewport = Template.bind({})
SmallViewport.args = {
  template: Mocks.MockTemplate,
  activeVersion: {
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
  data: {
    resources: [Mocks.MockWorkspaceResource, Mocks.MockWorkspaceResource2],
    versions: [Mocks.MockTemplateVersion],
    daus: Mocks.MockTemplateDAUResponse,
  },
}
SmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
