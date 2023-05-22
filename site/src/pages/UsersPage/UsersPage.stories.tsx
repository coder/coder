import type { Meta, StoryObj } from "@storybook/react"
import UsersPage from "./UsersPage"
import { UsersLayout } from "components/UsersLayout/UsersLayout"
import { DashboardPage } from "testHelpers/storybookHelpers"

const meta: Meta<typeof UsersPage> = {
  title: "pages/UsersPage",
  component: UsersPage,
}

export default meta
type Story = StoryObj<typeof UsersPage>

export const Admin: Story = {
  parameters: {
    layout: "fullscreen",
  },
  render: () => (
    <DashboardPage
      layout={<UsersLayout />}
      page={<UsersPage />}
      path="/users"
    />
  ),
}
