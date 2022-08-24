import { ComponentMeta, Story } from "@storybook/react"
import {
  assignableRole,
  MockAuditorRole,
  MockMemberRole,
  MockOwnerRole,
  MockTemplateAdminRole,
  MockUserAdminRole,
} from "../../testHelpers/renderHelpers"
import { RoleSelect, RoleSelectProps } from "./RoleSelect"

export default {
  title: "components/RoleSelect",
  component: RoleSelect,
} as ComponentMeta<typeof RoleSelect>

const Template: Story<RoleSelectProps> = (args) => <RoleSelect {...args} />

// Include 4 roles:
// - owner (disabled, not checked)
// - template admin (disabled, checked)
// - auditor (enabled, not checked)
// - user admin (enabled, checked)
export const Close = Template.bind({})
Close.args = {
  roles: [
    assignableRole(MockOwnerRole, false),
    assignableRole(MockTemplateAdminRole, false),
    assignableRole(MockAuditorRole, true),
    assignableRole(MockUserAdminRole, true),
  ],
  selectedRoles: [MockUserAdminRole, MockTemplateAdminRole, MockMemberRole],
}

export const Open = Template.bind({})
Open.args = {
  open: true,
  roles: [
    assignableRole(MockOwnerRole, false),
    assignableRole(MockTemplateAdminRole, false),
    assignableRole(MockAuditorRole, true),
    assignableRole(MockUserAdminRole, true),
  ],
  selectedRoles: [MockUserAdminRole, MockTemplateAdminRole, MockMemberRole],
}
