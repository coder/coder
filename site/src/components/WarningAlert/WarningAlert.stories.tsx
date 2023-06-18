import { Meta, StoryObj } from "@storybook/react"
import { WarningAlert } from "./WarningAlert"
import Button from "@mui/material/Button"

const meta: Meta<typeof WarningAlert> = {
  title: "components/WarningAlert",
  component: WarningAlert,
}

export default meta

type Story = StoryObj<typeof WarningAlert>

export const ExampleWithDismiss: Story = {
  args: {
    text: "This is a warning",
    dismissible: true,
  },
}

const ExampleAction = (
  <Button onClick={() => null} size="small" variant="text">
    Button
  </Button>
)

export const ExampleWithAction = {
  args: {
    text: "This is a warning",
    actions: [ExampleAction],
  },
}

export const ExampleWithActionAndDismiss = {
  args: {
    text: "This is a warning",
    actions: [ExampleAction],
    dismissible: true,
  },
}
