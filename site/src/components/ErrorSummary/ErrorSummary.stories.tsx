import { action } from "@storybook/addon-actions"
import { ComponentMeta, Story } from "@storybook/react"
import React from "react"
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
