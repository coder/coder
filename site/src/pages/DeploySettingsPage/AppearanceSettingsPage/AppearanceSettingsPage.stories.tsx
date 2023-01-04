import { ComponentMeta, Story } from "@storybook/react"
import AppearanceSettingsPage from "./AppearanceSettingsPage"

export default {
    title: "pages/AppearanceSettingsPage",
    component: AppearanceSettingsPage,
} as ComponentMeta<typeof AppearanceSettingsPage>

const Template: Story = (args) => <AppearanceSettingsPage {...args} />

export const Default = Template.bind({})
Default.args = {}
