import { Story } from "@storybook/react"
import { TemplateVersionParameter } from "api/typesGenerated"
import {
  RichParameterInput,
  RichParameterInputProps,
} from "./RichParameterInput"

export default {
  title: "components/RichParameterInput",
  component: RichParameterInput,
}

const Template: Story<RichParameterInputProps> = (
  args: RichParameterInputProps,
) => <RichParameterInput {...args} />

const createTemplateVersionParameter = (
  partial: Partial<TemplateVersionParameter>,
): TemplateVersionParameter => {
  return {
    name: "first_parameter",
    description: "This is first parameter.",
    type: "string",
    mutable: false,
    default_value: "default string",
    icon: "/icon/folder.svg",
    options: [],
    validation_error: "",
    validation_regex: "",
    validation_min: 0,
    validation_max: 0,
    validation_monotonic: "",

    ...partial,
  }
}

export const Basic = Template.bind({})
Basic.args = {
  initialValue: "initial-value",
  parameter: createTemplateVersionParameter({
    name: "project_name",
    description:
      "Customize the name of a Google Cloud project that will be created!",
  }),
}

export const NumberType = Template.bind({})
NumberType.args = {
  initialValue: "4",
  parameter: createTemplateVersionParameter({
    name: "number_parameter",
    type: "number",
    description: "Numeric parameter",
  }),
}

export const BooleanType = Template.bind({})
BooleanType.args = {
  initialValue: "false",
  parameter: createTemplateVersionParameter({
    name: "bool_parameter",
    type: "bool",
    description: "Boolean parameter",
  }),
}

export const OptionsType = Template.bind({})
OptionsType.args = {
  initialValue: "first_option",
  parameter: createTemplateVersionParameter({
    name: "options_parameter",
    type: "string",
    description: "Parameter with options",
    options: [
      {
        name: "First option",
        value: "first_option",
        description: "This is option 1",
        icon: "",
      },
      {
        name: "Second option",
        value: "second_option",
        description: "This is option 2",
        icon: "/icon/database.svg",
      },
      {
        name: "Third option",
        value: "third_option",
        description: "This is option 3",
        icon: "/icon/aws.png",
      },
    ],
  }),
}
