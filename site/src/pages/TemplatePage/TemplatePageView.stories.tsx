import { Story } from "@storybook/react"
import * as Mocks from "../../testHelpers/renderHelpers"
import { TemplatePageView, TemplatePageViewProps } from "./TemplatePageView"

export default {
  title: "pages/TemplatePageView",
  component: TemplatePageView,
}

const Template: Story<TemplatePageViewProps> = (args) => <TemplatePageView {...args} />

export const Example = Template.bind({})
Example.args = {
  template: Mocks.MockTemplate,
  activeTemplateVersion: Mocks.MockTemplateVersion,
  templateResources: [Mocks.MockWorkspaceResource, Mocks.MockWorkspaceResource2],
}

export const SmallViewport = Template.bind({})
SmallViewport.args = {
  template: Mocks.MockTemplate,
  activeTemplateVersion: Mocks.MockTemplateVersion,
  templateResources: [Mocks.MockWorkspaceResource, Mocks.MockWorkspaceResource2],
}
SmallViewport.parameters = {
  chromatic: { viewports: [600] },
}
