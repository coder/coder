import { Story } from "@storybook/react"
import React from "react"
import { CodeExample, CodeExampleProps } from "./CodeExample"

const sampleCode = `echo "Hello, world"`

export default {
  title: "CodeBlock/CodeExample",
  component: CodeExample,
  argTypes: {
    code: { control: "string", defaultValue: sampleCode },
  },
}

const Template: Story<CodeExampleProps> = (args: CodeExampleProps) => <CodeExample {...args} />

export const Example = Template.bind({})
Example.args = {
  code: sampleCode,
}
