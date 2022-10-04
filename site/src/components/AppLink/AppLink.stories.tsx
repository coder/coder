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
  appIcon: "/icon/code.svg",
  health: "healthy",
}

export const WithoutIcon = Template.bind({})
WithoutIcon.args = {
  userName: "developer",
  workspaceName: MockWorkspace.name,
  appName: "code-server",
  health: "healthy",
}

export const HealthDisabled = Template.bind({})
HealthDisabled.args = {
  userName: "developer",
  workspaceName: MockWorkspace.name,
  appName: "code-server",
  health: "disabled",
}

export const HealthInitializing = Template.bind({})
HealthInitializing.args = {
  userName: "developer",
  workspaceName: MockWorkspace.name,
  appName: "code-server",
  health: "initializing",
}

export const HealthUnhealthy = Template.bind({})
HealthUnhealthy.args = {
  userName: "developer",
  workspaceName: MockWorkspace.name,
  appName: "code-server",
  health: "unhealthy",
}
