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
} as ComponentMeta<typeof EditRolesButton>

const Template: Story<EditRolesButtonProps> = (args) => (
  <EditRolesButton {...args} />
)

export const Open = Template.bind({})
Open.args = {
  defaultIsOpen: true,
  roles: MockSiteRoles,
  selectedRoles: [MockUserAdminRole, MockOwnerRole],
}

export const Loading = Template.bind({})
Loading.args = {
  defaultIsOpen: true,
  roles: MockSiteRoles,
  isLoading: true,
  selectedRoles: [MockUserAdminRole, MockOwnerRole],
}
