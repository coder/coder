import { Story } from "@storybook/react"
import React from "react"
import { CodeBlock, CodeBlockProps } from "./index"

const sampleLines = `Successfully assigned coder/image-jcws7 to gke-master-workspaces-1-ef039342-cybd
Container image "gcr.io/coder-dogfood/master/coder-dev-ubuntu@sha256:80604e432c039daf558edfd7c01b9ea1f62f9c4b0f30aeab954efe11c404f8ee" already present on machine
Created container user
Started container user
Using user 'coder' with shell '/bin/bash'`.split('\n')

export default {
  title: "CodeBlock",
  component: CodeBlockProps,
  argTypes: {
    lines: { control: "object", defaultValue: sampleLines },
  },
}

const Template: Story<CodeBlockProps> = (args) => <CodeBlock {...args} />

export const Example = Template.bind({})
Example.args = {
  lines: sampleLines,
}
