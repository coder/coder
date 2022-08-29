import { ComponentMeta, Story } from "@storybook/react"
import { MockTemplate } from "../../testHelpers/entities"
import { TemplatesPageView, TemplatesPageViewProps } from "./TemplatesPageView"

export default {
  title: "pages/TemplatesPageView",
  component: TemplatesPageView,
} as ComponentMeta<typeof TemplatesPageView>

const Template: Story<TemplatesPageViewProps> = (args) => <TemplatesPageView {...args} />

export const AllStates = Template.bind({})
AllStates.args = {
  canCreateTemplate: true,
  templates: [
    MockTemplate,
    {
      ...MockTemplate,
      description: "🚀 Some magical template that does some magical things!",
      icon: "/icon/goland.svg",
    },
    {
      ...MockTemplate,
      workspace_owner_count: 150,
      description: "😮 Wow, this one has a bunch of usage!",
      icon: "",
    },
    {
      ...MockTemplate,
      description:
        "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. ",
    },
  ],
}

export const SmallViewport = Template.bind({})
SmallViewport.args = {
  ...AllStates.args,
}
SmallViewport.parameters = {
  chromatic: { viewports: [600] },
}

export const EmptyCanCreate = Template.bind({})
EmptyCanCreate.args = {
  canCreateTemplate: true,
}

export const EmptyCannotCreate = Template.bind({})
EmptyCannotCreate.args = {}
