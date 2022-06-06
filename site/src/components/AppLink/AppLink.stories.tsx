import { Story } from "@storybook/react"
import { MockWorkspace } from "../../testHelpers/renderHelpers"
import { AppLink, AppLinkProps } from "./AppLink"

export default {
  title: "components/AppLink",
  component: AppLink,
}

const Template: Story<AppLinkProps> = (args) => <AppLink {...args} />

export const WithIcon = Template.bind({})
WithIcon.args = {
  userName: "developer",
  workspaceName: MockWorkspace.name,
  appName: "code-server",
  appIcon: "/code.svg",
}

export const WithoutIcon = Template.bind({})
WithoutIcon.args = {
  userName: "developer",
  workspaceName: MockWorkspace.name,
  appName: "code-server",
}
