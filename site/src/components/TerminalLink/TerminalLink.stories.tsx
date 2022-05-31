import { Story } from "@storybook/react"
import { MockWorkspace } from "../../testHelpers/renderHelpers"
import { TerminalLink, TerminalLinkProps } from "./TerminalLink"

export default {
  title: "components/TerminalLink",
  component: TerminalLink,
}

const Template: Story<TerminalLinkProps> = (args) => <TerminalLink {...args} />

export const Example = Template.bind({})
Example.args = {
  workspaceName: MockWorkspace.name,
}
