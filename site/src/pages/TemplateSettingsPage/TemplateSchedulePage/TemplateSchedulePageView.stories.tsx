import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import { MockTemplate } from "testHelpers/entities"
import {
  TemplateSchedulePageView,
  TemplateSchedulePageViewProps,
} from "./TemplateSchedulePageView"

export default {
  title: "pages/TemplateSchedulePageView",
  component: TemplateSchedulePageView,
  args: {
    canSetMaxTTL: true,
    template: MockTemplate,
    onSubmit: action("onSubmit"),
    onCancel: action("cancel"),
  },
}

const Template: Story<TemplateSchedulePageViewProps> = (args) => (
  <TemplateSchedulePageView {...args} />
)

export const Example = Template.bind({})
Example.args = {}

export const CantSetMaxTTL = Template.bind({})
CantSetMaxTTL.args = {
  canSetMaxTTL: false,
}
