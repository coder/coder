import { Story } from "@storybook/react"
import { CliAuthToken, CliAuthTokenProps } from "./CliAuthToken"

export default {
  title: "components/CliAuthToken",
  component: CliAuthToken,
  argTypes: {
    sessionToken: { control: "text", defaultValue: "some-session-token" },
  },
}

const Template: Story<CliAuthTokenProps> = (args) => <CliAuthToken {...args} />

export const Example = Template.bind({})
Example.args = {}
