import { action } from "@storybook/addon-actions"
import { ComponentMeta, Story } from "@storybook/react"
import { ErrorSummary, ErrorSummaryProps } from "./ErrorSummary"

export default {
  title: "components/ErrorSummary",
  component: ErrorSummary,
} as ComponentMeta<typeof ErrorSummary>

const Template: Story<ErrorSummaryProps> = (args) => <ErrorSummary {...args} />

export const WithError = Template.bind({})
WithError.args = {
  error: new Error("Something went wrong!"),
}

export const WithRetry = Template.bind({})
WithRetry.args = {
  error: new Error("Failed to fetch something!"),
  retry: () => {
    action("retry")
  },
}

export const WithUndefined = Template.bind({})

export const WithDefaultMessage = Template.bind({})
WithDefaultMessage.args = {
  // Unknown error type
  error: {
    message: "Failed to fetch something!",
  },
  defaultMessage: "This is a default error message",
}

export const WithDismissible = Template.bind({})
WithDismissible.args = {
  error: {
    response: {
      data: {
        message: "Failed to fetch something!",
      },
    },
    isAxiosError: true,
  },
  dismissible: true,
}

export const WithDetails = Template.bind({})
WithDetails.args = {
  error: {
    response: {
      data: {
        message: "Failed to fetch something!",
        detail: "The resource you requested does not exist in the database.",
      },
    },
    isAxiosError: true,
  },
  dismissible: true,
}
