import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import * as Mocks from "../../testHelpers/renderHelpers"
import { TemplateSettingsPageView, TemplateSettingsPageViewProps } from "./TemplateSettingsPageView"

export default {
  title: "pages/TemplateSettingsPageView",
  component: TemplateSettingsPageView,
}

const Template: Story<TemplateSettingsPageViewProps> = (args) => (
  <TemplateSettingsPageView {...args} />
)

export const Example = Template.bind({})
Example.args = {
  template: Mocks.MockTemplate,
  onSubmit: action("onSubmit"),
  onCancel: action("cancel"),
}
