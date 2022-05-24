import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { ParameterSchema } from "../../api/typesGenerated"
import { MockTemplate } from "../../testHelpers/entities"
import { CreateWorkspacePageView, CreateWorkspacePageViewProps } from "./CreateWorkspacePageView"

const createParameterSchema = (partial: Partial<ParameterSchema>): ParameterSchema => {
  return {
    id: "000000",
    job_id: "000000",
    allow_override_destination: false,
    allow_override_source: true,
    created_at: "",
    default_destination_scheme: "none",
    default_refresh: "",
    default_source_scheme: "data",
    default_source_value: "default-value",
    name: "parameter name",
    description: "Some description!",
    redisplay_value: false,
    validation_condition: "",
    validation_contains: [],
    validation_error: "",
    validation_type_system: "",
    validation_value_type: "",
    ...partial,
  }
}

export default {
  title: "pages/CreateWorkspacePageView",
  component: CreateWorkspacePageView,
} as ComponentMeta<typeof CreateWorkspacePageView>

const Template: Story<CreateWorkspacePageViewProps> = (args) => <CreateWorkspacePageView {...args} />

export const NoParameters = Template.bind({})
NoParameters.args = {
  templates: [MockTemplate],
  selectedTemplate: MockTemplate,
  templateSchema: [],
}

export const Parameters = Template.bind({})
Parameters.args = {
  templates: [MockTemplate],
  selectedTemplate: MockTemplate,
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
