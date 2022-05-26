import { Story } from "@storybook/react"
import React from "react"
import { PortForwardDropdown, PortForwardDropdownProps } from "./PortForwardDropdown"

export default {
  title: "components/PortForwardDropdown",
  component: PortForwardDropdown,
}

const Template: Story<PortForwardDropdownProps> = (args: PortForwardDropdownProps) => (
  <PortForwardDropdown anchorEl={document.body} urlFormatter={urlFormatter} open {...args} />
)

const urlFormatter = (port: number | string): string => {
  return `https://${port}--user--workspace.coder.com`
}

export const Error = Template.bind({})
Error.args = {
  netstat: {
    error: "Unable to get listening ports",
  },
}

export const Loading = Template.bind({})
Loading.args = {}

export const None = Template.bind({})
None.args = {
  netstat: {
    ports: [],
  },
}

export const Excluded = Template.bind({})
Excluded.args = {
  netstat: {
    ports: [
      {
        name: "sshd",
        port: 22,
      },
    ],
  },
}

export const Single = Template.bind({})
Single.args = {
  netstat: {
    ports: [
      {
        name: "code-server",
        port: 8080,
      },
    ],
  },
}

export const Multiple = Template.bind({})
Multiple.args = {
  netstat: {
    ports: [
      {
        name: "code-server",
        port: 8080,
      },
      {
        name: "coder",
        port: 8000,
      },
      {
        name: "coder",
        port: 3000,
      },
      {
        name: "node",
        port: 8001,
      },
      {
        name: "sshd",
        port: 22,
      },
    ],
  },
}
