import { Story } from "@storybook/react"
import { CodeExample, CodeExampleProps } from "./CodeExample"

const sampleCode = `echo "Hello, world"`

export default {
  title: "components/CodeExample",
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
