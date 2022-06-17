import { Story } from "@storybook/react"
import { Footer, FooterProps } from "./Footer"

export default {
  title: "components/Footer",
  component: Footer,
}

const Template: Story<FooterProps> = (args) => <Footer {...args} />

export const Example = Template.bind({})
Example.args = {
  buildInfo: {
    external_url: "",
    version: "test-1.2.3",
  },
}
