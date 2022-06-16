import { Story } from "@storybook/react"
import { Footer } from "./Footer"

export default {
  title: "components/Footer",
  component: Footer,
}

const Template: Story = () => <Footer />

export const Example = Template.bind({})
Example.args = {}
