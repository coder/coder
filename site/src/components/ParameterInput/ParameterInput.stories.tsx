import { Story } from "@storybook/react"
import { ParameterSchema } from "../../api/typesGenerated"
import { ParameterInput, ParameterInputProps } from "./ParameterInput"

export default {
  title: "components/ParameterInput",
  component: ParameterInput,
}

const Template: Story<ParameterInputProps> = (args: ParameterInputProps) => (
  <ParameterInput {...args} />
)

const createParameterSchema = (
  partial: Partial<ParameterSchema>,
): ParameterSchema => {
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

export const Basic = Template.bind({})
Basic.args = {
  schema: createParameterSchema({
    name: "project_name",
    description:
      "Customize the name of a Google Cloud project that will be created!",
  }),
}

export const Boolean = Template.bind({})
Boolean.args = {
  schema: createParameterSchema({
    name: "disable_docker",
    description: "Disable Docker?",
    validation_value_type: "bool",
    default_source_value: "false",
  }),
}

export const Contains = Template.bind({})
Contains.args = {
  schema: createParameterSchema({
    name: "region",
    default_source_value: "🏈 US Central",
    description: "Where would you like your workspace to live?",
    validation_contains: [
      "🏈 US Central",
      "⚽ Brazil East",
      "💶 EU West",
      "🦘 Australia South",
    ],
  }),
}
