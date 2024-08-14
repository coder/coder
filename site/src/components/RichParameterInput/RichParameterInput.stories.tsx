import type { Meta, StoryObj } from "@storybook/react";
import type { TemplateVersionParameter } from "api/typesGenerated";
import { chromatic } from "testHelpers/chromatic";
import { RichParameterInput } from "./RichParameterInput";

const meta: Meta<typeof RichParameterInput> = {
  title: "components/RichParameterInput",
  parameters: { chromatic },
  component: RichParameterInput,
};

export default meta;
type Story = StoryObj<typeof RichParameterInput>;

const createTemplateVersionParameter = (
  partial: Partial<TemplateVersionParameter>,
): TemplateVersionParameter => {
  return {
    name: "first_parameter",
    description: "This is first parameter.",
    type: "string",
    mutable: true,
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
  };
};

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
};

export const Optional: Story = {
  args: {
    value: "initial-value",
    id: "project_name",
    parameter: createTemplateVersionParameter({
      required: false,
      name: "project_name",
      description:
        "Customize the name of a Google Cloud project that will be created!",
    }),
  },
};

export const Immutable: Story = {
  args: {
    value: "initial-value",
    id: "project_name",
    parameter: createTemplateVersionParameter({
      mutable: false,
      name: "project_name",
      description:
        "Customize the name of a Google Cloud project that will be created!",
    }),
  },
};

export const WithError: Story = {
  args: {
    id: "number_parameter",
    parameter: createTemplateVersionParameter({
      name: "number_parameter",
      type: "number",
      description: "Numeric parameter",
      default_value: "",
    }),
    error: true,
    helperText: "Number must be greater than 5",
  },
};

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
};

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
};

export const Options: Story = {
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
          description: "",
          icon: "/icon/fedora.svg",
        },
        {
          name: "Second option",
          value: "second_option",
          description: "",
          icon: "/icon/database.svg",
        },
        {
          name: "Third option",
          value: "third_option",
          description: "",
          icon: "/icon/aws.svg",
        },
      ],
    }),
  },
};

export const OptionsWithDescriptions: Story = {
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
          description: "This is a short description.",
          icon: "/icon/fedora.svg",
        },
        {
          name: "Second option",
          value: "second_option",
          description:
            "This description is a little bit longer, but still not very long.",
          icon: "/icon/database.svg",
        },
        {
          name: "Third option",
          value: "third_option",
          description: `
In this description, we will explore the various ways in which this description
is a big long boy. We'll discuss such things as, lots of words wow it's long, and
boy howdy that's a number of sentences that this description contains. By the conclusion
of this essay, I hope to reveal to you, the reader, that this description is just an
absolute chonker. Just way longer than it actually needs to be. Absolutely massive.
Very big.

> Wow, that description is straight up large. –Some guy, probably
`,
          icon: "/icon/aws.svg",
        },
      ],
    }),
  },
};

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
};

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
};

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
};

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
};

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
};

// Smaller version of the components. Used in popovers.

export const SmallBasic: Story = {
  args: {
    ...Basic.args,
    size: "small",
  },
};

export const SmallNumberType: Story = {
  args: {
    ...NumberType.args,
    size: "small",
  },
};

export const SmallBooleanType: Story = {
  args: {
    ...BooleanType.args,
    size: "small",
  },
};

export const SmallOptions: Story = {
  args: {
    ...Options.args,
    size: "small",
  },
};

export const SmallOptionsWithDescriptions: Story = {
  args: {
    ...OptionsWithDescriptions.args,
    size: "small",
  },
};

export const SmallListStringType: Story = {
  args: {
    ...ListStringType.args,
    size: "small",
  },
};

export const SmallIconLabel: Story = {
  args: {
    ...IconLabel.args,
    size: "small",
  },
};

export const SmallNoDescription: Story = {
  args: {
    ...NoDescription.args,
    size: "small",
  },
};

export const SmallBasicWithDisplayName: Story = {
  args: {
    ...BasicWithDisplayName.args,
    size: "small",
  },
};
