import { Alert } from "./Alert"
import Button from "@mui/material/Button"
import { mockApiError } from "testHelpers/entities"
import Link from "@mui/material/Link"
import { getErrorMessage } from "api/errors"
import type { Meta, StoryObj } from "@storybook/react"
import { action } from "@storybook/addon-actions"

const meta: Meta<typeof Alert> = {
  title: "components/Alert",
  component: Alert,
  args: {
    severity: "error",
  },
}

export default meta
type Story = StoryObj<typeof Alert>

const ExampleAction = (
  <Button onClick={() => null} size="small">
    Button
  </Button>
)

const mockError = mockApiError({
  message: "Email or password was invalid",
  detail: "Password is invalid",
})

export const Warning: Story = {
  args: {
    children: "This is a warning",
    severity: "warning",
  },
}

export const ErrorWithDefaultMessage: Story = {
  args: {
    children: "This is an error",
    severity: "error",
  },
}

export const ErrorWithErrorMessage: Story = {
  args: {
    children: getErrorMessage(mockError, "Error default message"),
    severity: "error",
  },
}

export const WarningWithDismiss: Story = {
  args: {
    children: "This is a warning",
    dismissible: true,
    severity: "warning",
  },
}

export const ErrorWithDismiss: Story = {
  args: {
    children: getErrorMessage(mockError, "Default error message"),
    dismissible: true,
    severity: "error",
  },
}
export const WarningWithAction: Story = {
  args: {
    children: "This is a warning",
    actions: [ExampleAction],
    severity: "warning",
  },
}

export const ErrorWithAction: Story = {
  args: {
    children: getErrorMessage(mockError, "Default error message"),
    actions: [ExampleAction],
    severity: "error",
  },
}

export const WarningWithActionAndDismiss: Story = {
  args: {
    children: "This is a warning",
    actions: [ExampleAction],
    dismissible: true,
    severity: "warning",
  },
}

export const ErrorWithActionAndDismiss: Story = {
  args: {
    children: getErrorMessage(mockError, "Default error message"),
    actions: [ExampleAction],
    dismissible: true,
    severity: "error",
  },
}

export const ErrorWithRetry: Story = {
  args: {
    children: getErrorMessage(mockError, "Default error message"),
    onRetry: action("retry"),
    dismissible: true,
    severity: "error",
  },
}

export const ErrorWithActionRetryAndDismiss: Story = {
  args: {
    children: getErrorMessage(mockError, "Default error message"),
    actions: [ExampleAction],
    onRetry: action("retry"),
    dismissible: true,
    severity: "error",
  },
}

export const ErrorAsWarning: Story = {
  args: {
    children: getErrorMessage(mockError, "Default error message"),
    severity: "warning",
  },
}

export const WithChildren: Story = {
  args: {
    children: (
      <div>
        This is a message with a <Link href="#">link</Link>
      </div>
    ),
  },
}
