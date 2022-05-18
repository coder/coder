import { Story } from "@storybook/react"
import React from "react"
import { BorderedMenuRow } from "../BorderedMenuRow/BorderedMenuRow"
import { BuildingIcon } from "../Icons/BuildingIcon"
import { UsersOutlinedIcon } from "../Icons/UsersOutlinedIcon"
import { BorderedMenu, BorderedMenuProps } from "./BorderedMenu"

export default {
  title: "components/BorderedMenu",
  component: BorderedMenu,
}

const Template: Story<BorderedMenuProps> = (args: BorderedMenuProps) => (
  <BorderedMenu {...args}>
    <BorderedMenuRow title="Item 1" description="Here's a description" Icon={BuildingIcon} />
    <BorderedMenuRow active title="Item 2" description="This BorderedMenuRow is active" Icon={UsersOutlinedIcon} />
  </BorderedMenu>
)

export const AdminVariant = Template.bind({})
AdminVariant.args = {
  variant: "admin-dropdown",
  open: true,
}

export const UserVariant = Template.bind({})
UserVariant.args = {
  variant: "user-dropdown",
  open: true,
}
