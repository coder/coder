import { Story } from "@storybook/react";
import { MockUser, MockUser2 } from "../../../testHelpers/entities";
import { NavbarView, NavbarViewProps } from "./NavbarView";

export default {
  title: "components/NavbarView",
  component: NavbarView,
  argTypes: {
    onSignOut: { action: "Sign Out" },
  },
};

const Template: Story<NavbarViewProps> = (args: NavbarViewProps) => (
  <NavbarView {...args} />
);

export const ForAdmin = Template.bind({});
ForAdmin.args = {
  user: MockUser,
  onSignOut: () => {
    return Promise.resolve();
  },
};

export const ForMember = Template.bind({});
ForMember.args = {
  user: MockUser2,
  onSignOut: () => {
    return Promise.resolve();
  },
};

export const SmallViewport = Template.bind({});
SmallViewport.args = {
  user: MockUser,
  onSignOut: () => {
    return Promise.resolve();
  },
};
SmallViewport.parameters = {
  viewport: {
    defaultViewport: "tablet",
  },
  chromatic: { viewports: [420] },
};
