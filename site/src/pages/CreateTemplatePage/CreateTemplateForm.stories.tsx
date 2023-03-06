import { ComponentMeta, Story } from "@storybook/react"
import {
  MockParameterSchemas,
  MockTemplateExample,
  MockTemplateVersionVariable1,
  MockTemplateVersionVariable2,
  MockTemplateVersionVariable3,
  MockTemplateVersionVariable4,
  MockTemplateVersionVariable5,
} from "testHelpers/entities"
import {
  CreateTemplateForm,
  CreateTemplateFormProps,
} from "./CreateTemplateForm"

export default {
  title: "components/CreateTemplateForm",
  component: CreateTemplateForm,
  args: {
    isSubmitting: false,
  },
} as ComponentMeta<typeof CreateTemplateForm>

const Template: Story<CreateTemplateFormProps> = (args) => (
  <CreateTemplateForm {...args} />
)

export const Initial = Template.bind({})
Initial.args = {}

export const WithStarterTemplate = Template.bind({})
WithStarterTemplate.args = {
  starterTemplate: MockTemplateExample,
}

export const WithParameters = Template.bind({})
WithParameters.args = {
  parameters: MockParameterSchemas,
}

export const WithVariables = Template.bind({})
WithVariables.args = {
  variables: [
    MockTemplateVersionVariable1,
    MockTemplateVersionVariable2,
    MockTemplateVersionVariable3,
    MockTemplateVersionVariable4,
    MockTemplateVersionVariable5,
  ],
}
