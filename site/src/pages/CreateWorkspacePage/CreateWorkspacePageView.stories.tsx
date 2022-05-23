import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { createParameterSchema } from "../../components/ParameterInput/ParameterInput.stories"
import { MockTemplate } from "../../testHelpers/entities"
import { CreateWorkspacePageView, CreateWorkspacePageViewProps } from "./CreateWorkspacePageView"

export default {
  title: "pages/CreateWorkspacePageView",
  component: CreateWorkspacePageView,
} as ComponentMeta<typeof CreateWorkspacePageView>

const Template: Story<CreateWorkspacePageViewProps> = (args) => <CreateWorkspacePageView {...args} />

export const NoParameters = Template.bind({})
NoParameters.args = {
  template: MockTemplate,
  templateSchema: [],
}

export const Parameters = Template.bind({})
Parameters.args = {
  template: MockTemplate,
  templateSchema: [
    createParameterSchema({
      name: "region",
      default_source_value: "🏈 US Central",
      description: "Where would you like your workspace to live?",
      validation_contains: ["🏈 US Central", "⚽ Brazil East", "💶 EU West", "🦘 Australia South"],
    }),
    createParameterSchema({
      name: "instance_size",
      default_source_value: "Big",
      description: "How large should you instance be?",
      validation_contains: ["Small", "Medium", "Big"],
    }),
  ],
}
