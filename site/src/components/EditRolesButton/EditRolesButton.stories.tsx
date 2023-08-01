import { ComponentMeta, Story } from "@storybook/react"
import {
  MockOwnerRole,
  MockSiteRoles,
  MockUserAdminRole,
} from "testHelpers/entities"
import { EditRolesButtonProps, EditRolesButton } from "./EditRolesButton"

export default {
  title: "components/EditRolesButton",
  component: EditRolesButton,
  argTypes: {
    defaultIsOpen: {
      defaultValue: true,
    },
  },
} as ComponentMeta<typeof EditRolesButton>

const Template: Story<EditRolesButtonProps> = (args) => (
  <EditRolesButton {...args} />
)

export const Open = Template.bind({})
Open.args = {
  roles: MockSiteRoles,
  selectedRoles: [MockUserAdminRole, MockOwnerRole],
}
Open.parameters = {
  chromatic: { delay: 300 },
}

export const Loading = Template.bind({})
Loading.args = {
  isLoading: true,
  roles: MockSiteRoles,
  selectedRoles: [MockUserAdminRole, MockOwnerRole],
  userLoginType: "password",
  oidcRoleSync: false,
}
Loading.parameters = {
  chromatic: { delay: 300 },
}
