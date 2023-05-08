import { Story } from "@storybook/react"
import {
  MockTemplate,
  MockTemplateDAUResponse,
  MockTemplateVersion,
  MockWorkspaceResource,
  MockWorkspaceResource2,
} from "testHelpers/entities"
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
  template: MockTemplate,
  activeVersion: MockTemplateVersion,
  data: {
    resources: [MockWorkspaceResource, MockWorkspaceResource2],
    daus: MockTemplateDAUResponse,
  },
}

export const NoIcon = Template.bind({})
NoIcon.args = {
  template: { ...MockTemplate, icon: "" },
  activeVersion: MockTemplateVersion,
  data: {
    resources: [MockWorkspaceResource, MockWorkspaceResource2],
    daus: MockTemplateDAUResponse,
  },
}

export const SmallViewport = Template.bind({})
SmallViewport.args = {
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
  data: {
    resources: [MockWorkspaceResource, MockWorkspaceResource2],
    daus: MockTemplateDAUResponse,
  },
}
SmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
