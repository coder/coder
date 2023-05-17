import { Alert } from "./Alert"
import Button from "@mui/material/Button"
import { mockApiError } from "testHelpers/entities"
import type { Meta, StoryObj } from "@storybook/react"
import { action } from "@storybook/addon-actions"
import { ErrorAlert } from "./ErrorAlert"

const mockError = mockApiError({
  message: "Email or password was invalid",
  detail: "Password is invalid",
})

const meta: Meta<typeof ErrorAlert> = {
  title: "components/ErrorAlert",
  component: ErrorAlert,
  args: {
    error: mockError,
  },
}

export default meta
type Story = StoryObj<typeof Alert>

const ExampleAction = (
  <Button onClick={() => null} size="small">
    Button
  </Button>
)

export const WithDismiss: Story = {
  args: {
    dismissible: true,
  },
}

export const WithAction: Story = {
  args: {
    actions: [ExampleAction],
  },
}

export const WithActionAndDismiss: Story = {
  args: {
    actions: [ExampleAction],
    dismissible: true,
  },
}

export const WithRetry: Story = {
  args: {
    onRetry: action("retry"),
    dismissible: true,
  },
}

export const WithActionRetryAndDismiss: Story = {
  args: {
    actions: [ExampleAction],
    onRetry: action("retry"),
    dismissible: true,
  },
}
