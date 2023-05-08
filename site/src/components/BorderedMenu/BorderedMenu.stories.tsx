import { Story } from "@storybook/react"
import { BorderedMenuRow } from "../BorderedMenuRow/BorderedMenuRow"
import { BorderedMenu, BorderedMenuProps } from "./BorderedMenu"

export default {
  title: "components/BorderedMenu",
  component: BorderedMenu,
}

const Template: Story<BorderedMenuProps> = (args: BorderedMenuProps) => (
  <BorderedMenu {...args}>
    <BorderedMenuRow
      title="Item 1"
      description="Here's a description"
      path="/"
    />
    <BorderedMenuRow
      active
      title="Item 2"
      description="This BorderedMenuRow is active"
      path="/"
    />
  </BorderedMenu>
)

export const UserVariant = Template.bind({})
UserVariant.args = {
  variant: "user-dropdown",
  open: true,
}
