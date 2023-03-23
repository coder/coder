import { action } from "@storybook/addon-actions"
import { Story } from "@storybook/react"
import * as Mocks from "../../../testHelpers/renderHelpers"
import {
  TemplateSchedulePageView,
  TemplateSchedulePageViewProps,
} from "./TemplateSchedulePageView"

export default {
  title: "pages/TemplateSchedulePageView",
  component: TemplateSchedulePageView,
  args: {
    canSetMaxTTL: true,
    template: Mocks.MockTemplate,
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
