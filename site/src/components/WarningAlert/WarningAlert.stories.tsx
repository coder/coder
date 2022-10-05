import { Story } from "@storybook/react"
import { WarningAlert, WarningAlertProps } from "./WarningAlert"
import Button from "@material-ui/core/Button"

export default {
  title: "components/WarningAlert",
  component: WarningAlert,
}

const Template: Story<WarningAlertProps> = (args) => <WarningAlert {...args} />

export const ExampleWithClose = Template.bind({})
ExampleWithClose.args = {
  text: "This is a warning",
}

const ExampleAction = (
  <Button color="inherit" onClick={() => null} size="small">
    Button
  </Button>
)

export const ExampleWithAction = Template.bind({})
ExampleWithAction.args = {
  text: "This is a warning",
  action: ExampleAction,
}
