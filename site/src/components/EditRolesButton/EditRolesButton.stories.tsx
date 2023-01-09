import { within, waitFor, expect } from '@storybook/testing-library';
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
  defaultIsOpen: true,
}
Open.play = async ({ canvasElement }) => {
  // Assigns canvas to the component root element
  const canvas = within(canvasElement);

  //   Wait for the below assertion not throwing an error anymore (default timeout is 1000ms)
  //👇 This is especially useful when you have an asynchronous action or component that you want to wait for
  await waitFor(() => {
    //👇 This assertion will pass if a DOM element with the matching id exists
    expect(canvas.getByTitle("Available roles")).toBeInTheDocument();
  });
};

export const Loading = Template.bind({})
Loading.args = {
  isLoading: true,
  roles: MockSiteRoles,
  selectedRoles: [MockUserAdminRole, MockOwnerRole],
}
Loading.parameters = {
  chromatic: { delay: 300 },
}
