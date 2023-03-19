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
    validation_monotonic: "increasing",
    description_plaintext: "",
    required: true,
    ...partial,
  }
}

export const Basic = Template.bind({})
Basic.args = {
  initialValue: "initial-value",
  id: "project_name",
  parameter: createTemplateVersionParameter({
    name: "project_name",
    description:
      "Customize the name of a Google Cloud project that will be created!",
  }),
}

export const NumberType = Template.bind({})
NumberType.args = {
  initialValue: "4",
  id: "number_parameter",
  parameter: createTemplateVersionParameter({
    name: "number_parameter",
    type: "number",
    description: "Numeric parameter",
  }),
}

export const BooleanType = Template.bind({})
BooleanType.args = {
  initialValue: "false",
  id: "bool_parameter",
  parameter: createTemplateVersionParameter({
    name: "bool_parameter",
    type: "bool",
    description: "Boolean parameter",
  }),
}

export const OptionsType = Template.bind({})
OptionsType.args = {
  initialValue: "first_option",
  id: "options_parameter",
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

export const ListStringType = Template.bind({})
ListStringType.args = {
  initialValue: JSON.stringify(["first", "second", "third"]),
  id: "list_string_parameter",
  parameter: createTemplateVersionParameter({
    name: "list_string_parameter",
    type: "list(string)",
    description: "List string parameter",
  }),
}

export const IconLabel = Template.bind({})
IconLabel.args = {
  initialValue: "initial-value",
  id: "project_name",
  parameter: createTemplateVersionParameter({
    name: "project_name",
    description:
      "Customize the name of a Google Cloud project that will be created!",
    icon: "/emojis/1f30e.png",
  }),
}

export const NoDescription = Template.bind({})
NoDescription.args = {
  initialValue: "",
  id: "region",
  parameter: createTemplateVersionParameter({
    name: "Region",
    description: "",
    description_plaintext: "",
    type: "string",
    mutable: false,
    default_value: "",
    icon: "/emojis/1f30e.png",
    options: [
      {
        name: "Pittsburgh",
        description: "",
        value: "us-pittsburgh",
        icon: "/emojis/1f1fa-1f1f8.png",
      },
      {
        name: "Helsinki",
        description: "",
        value: "eu-helsinki",
        icon: "/emojis/1f1eb-1f1ee.png",
      },
      {
        name: "Sydney",
        description: "",
        value: "ap-sydney",
        icon: "/emojis/1f1e6-1f1fa.png",
      },
    ],
  }),
}

export const DescriptionWithLinks = Template.bind({})
DescriptionWithLinks.args = {
  initialValue: "",
  id: "coder-repository-directory",
  parameter: createTemplateVersionParameter({
    name: "Coder Repository Directory",
    description:
      "The directory specified will be created and [coder/coder](https://github.com/coder/coder) will be automatically cloned into it 🪄.",
    description_plaintext:
      "The directory specified will be created and coder/coder (https://github.com/coder/coder) will be automatically cloned into it 🪄.",
    type: "string",
    mutable: true,
    default_value: "~/coder",
    icon: "",
    options: [],
  }),
}
