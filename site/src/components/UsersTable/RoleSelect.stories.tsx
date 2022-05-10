import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { RoleSelect, RoleSelectProps } from "./RoleSelect"

export default {
  title: "components/RoleSelect",
  component: RoleSelect,
} as ComponentMeta<typeof RoleSelect>

const Template: Story<RoleSelectProps> = (args) => <RoleSelect {...args} />

export const Close = Template.bind({})
Close.args = {
  roles: [
    {
      name: "admin",
      display_name: "Admin",
    },
    {
      name: "auditor",
      display_name: "Auditor",
    },
    {
      name: "member",
      display_name: "Member",
    },
  ],
  selectedRoles: [
    {
      name: "admin",
      display_name: "Admin",
    },
    {
      name: "member",
      display_name: "Member",
    },
  ],
}

export const Open = Template.bind({})
Open.args = {
  open: true,
  roles: [
    {
      name: "admin",
      display_name: "Admin",
    },
    {
      name: "auditor",
      display_name: "Auditor",
    },
    {
      name: "member",
      display_name: "Member",
    },
  ],
  selectedRoles: [
    {
      name: "admin",
      display_name: "Admin",
    },
    {
      name: "member",
      display_name: "Member",
    },
  ],
}
