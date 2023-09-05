import { TemplateVersionParameter } from "api/typesGenerated"
import { RichParameterInput } from "./RichParameterInput"
import type { Meta, StoryObj } from "@storybook/react"

const meta: Meta<typeof RichParameterInput> = {
  title: "components/RichParameterInput",
  component: RichParameterInput,
}

export default meta
type Story = StoryObj<typeof RichParameterInput>

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
    ephemeral: false,
    ...partial,
  }
}

export const Basic: Story = {
  args: {
    value: "initial-value",
    id: "project_name",
    parameter: createTemplateVersionParameter({
      name: "project_name",
      description:
        "Customize the name of a Google Cloud project that will be created!",
    }),
  },
}

export const NumberType: Story = {
  args: {
    value: "4",
    id: "number_parameter",
    parameter: createTemplateVersionParameter({
      name: "number_parameter",
      type: "number",
      description: "Numeric parameter",
    }),
  },
}

export const BooleanType: Story = {
  args: {
    value: "false",
    id: "bool_parameter",
    parameter: createTemplateVersionParameter({
      name: "bool_parameter",
      type: "bool",
      description: "Boolean parameter",
    }),
  },
}

export const OptionsType: Story = {
  args: {
    value: "first_option",
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
  },
}

export const ListStringType: Story = {
  args: {
    value: JSON.stringify(["first", "second", "third"]),
    id: "list_string_parameter",
    parameter: createTemplateVersionParameter({
      name: "list_string_parameter",
      type: "list(string)",
      description: "List string parameter",
    }),
  },
}

export const IconLabel: Story = {
  args: {
    value: "initial-value",
    id: "project_name",
    parameter: createTemplateVersionParameter({
      name: "project_name",
      description:
        "Customize the name of a Google Cloud project that will be created!",
      icon: "/emojis/1f30e.png",
    }),
  },
}

export const NoDescription: Story = {
  args: {
    value: "",
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
  },
}

export const DescriptionWithLinks: Story = {
  args: {
    value: "",
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
  },
}

export const BasicWithDisplayName: Story = {
  args: {
    value: "initial-value",
    id: "project_name",
    parameter: createTemplateVersionParameter({
      name: "project_name",
      display_name: "Project Name",
      description:
        "Customize the name of a Google Cloud project that will be created!",
    }),
  },
}

// Smaller version of the components. Used in popovers.

export const SmallBasic: Story = {
  args: {
    ...Basic.args,
    size: "small",
  },
}

export const SmallNumberType: Story = {
  args: {
    ...NumberType.args,
    size: "small",
  },
}

export const SmallBooleanType: Story = {
  args: {
    ...BooleanType.args,
    size: "small",
  },
}

export const SmallOptionsType: Story = {
  args: {
    ...OptionsType.args,
    size: "small",
  },
}

export const SmallListStringType: Story = {
  args: {
    ...ListStringType.args,
    size: "small",
  },
}

export const SmallIconLabel: Story = {
  args: {
    ...IconLabel.args,
    size: "small",
  },
}

export const SmallNoDescription: Story = {
  args: {
    ...NoDescription.args,
    size: "small",
  },
}

export const SmallBasicWithDisplayName: Story = {
  args: {
    ...BasicWithDisplayName.args,
    size: "small",
  },
}
