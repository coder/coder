import { Story } from "@storybook/react"
import { CodeBlock, CodeBlockProps } from "./CodeBlock"

const sampleLines = `Successfully assigned coder/image-jcws7 to cluster-1
Container image "gcr.io/coder-dogfood/master/coder-dev-ubuntu@sha256" already present on machine
Created container user
Started container user
Using user 'coder' with shell '/bin/bash'`.split("\n")

export default {
  title: "components/CodeBlock",
  component: CodeBlock,
  argTypes: {
    lines: { control: "text", defaultValue: sampleLines },
  },
}

const Template: Story<CodeBlockProps> = (args: CodeBlockProps) => (
  <CodeBlock {...args} />
)

export const Example = Template.bind({})
Example.args = {
  lines: sampleLines,
}
