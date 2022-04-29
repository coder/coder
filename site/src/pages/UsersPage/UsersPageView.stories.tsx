import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockUser, MockUser2 } from "../../testHelpers"
import { UsersPageView, UsersPageViewProps } from "./UsersPageView"

export default {
  title: "pages/UsersPageView",
  component: UsersPageView,
} as ComponentMeta<typeof UsersPageView>

const Template: Story<UsersPageViewProps> = (args) => <UsersPageView {...args} />

export const Ready = Template.bind({})
Ready.args = {
  users: [MockUser, MockUser2],
}
export const Empty = Template.bind({})
Empty.args = {
  users: [],
}
