import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
import { MockUser, MockUser2 } from "../../test_helpers"
import { UsersPageView, UsersPageViewProps } from "./UsersPageView"

export default {
  title: "pages/UsersPageView",
  component: UsersPageView,
} as ComponentMeta<typeof UsersPageView>

const Template: Story<UsersPageViewProps> = (args) => <UsersPageView {...args} />

export const Ready = Template.bind({})
Ready.args = {
  users: [
    MockUser,
    MockUser2
  ],
  pager: {
    total: 2,
    after: "123",
    before: "456",
    limit: 10
  }
}
export const Loading = Template.bind({})
Loading.args = {
  users: [
  ]
}
