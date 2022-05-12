import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockAdminRole, MockMemberRole, MockSiteRoles } from "../../testHelpers/renderHelpers"
import { RoleSelect, RoleSelectProps } from "./RoleSelect"

export default {
  title: "components/RoleSelect",
  component: RoleSelect,
} as ComponentMeta<typeof RoleSelect>

const Template: Story<RoleSelectProps> = (args) => <RoleSelect {...args} />

export const Close = Template.bind({})
Close.args = {
  roles: MockSiteRoles,
  selectedRoles: [MockAdminRole, MockMemberRole],
}

export const Open = Template.bind({})
Open.args = {
  open: true,
  roles: MockSiteRoles,
  selectedRoles: [MockAdminRole, MockMemberRole],
}
