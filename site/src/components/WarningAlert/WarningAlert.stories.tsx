import { Story } from "@storybook/react"
import { WarningAlert, WarningAlertProps } from "./WarningAlert"
import Button from "@material-ui/core/Button"

export default {
  title: "components/WarningAlert",
  component: WarningAlert,
}

const Template: Story<WarningAlertProps> = (args) => <WarningAlert {...args} />

export const ExampleWithDismiss = Template.bind({})
ExampleWithDismiss.args = {
  text: "This is a warning",
  dismissible: true,
}

const ExampleAction = (
  <Button onClick={() => null} size="small">
    Button
  </Button>
)

export const ExampleWithAction = Template.bind({})
ExampleWithAction.args = {
  text: "This is a warning",
  actions: [ExampleAction],
}

export const ExampleWithActionAndDismiss = Template.bind({})
ExampleWithActionAndDismiss.args = {
  text: "This is a warning",
  actions: [ExampleAction],
  dismissible: true,
}
