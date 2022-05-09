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

export const WithUndefined = Template.bind({})
