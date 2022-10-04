import { Story } from "@storybook/react"
import { WarningSummary, WarningSummaryProps } from "./WarningSummary"

export default {
  title: "components/WarningSummary",
  component: WarningSummary,
}

const Template: Story<WarningSummaryProps> = (args) => <WarningSummary {...args} />

export const Example = Template.bind({})
Example.args = {
  warningString: "This is a warning",
}
