import { Story } from "@storybook/react"
import { AlertBanner } from "./AlertBanner"
import Button from "@material-ui/core/Button"
import { makeMockApiError } from "testHelpers/entities"
import { AlertBannerProps } from "./alertTypes"
import Link from "@material-ui/core/Link"

export default {
  title: "components/AlertBanner",
  component: AlertBanner,
}

const ExampleAction = (
  <Button onClick={() => null} size="small">
    Button
  </Button>
)

const mockError = makeMockApiError({
  message: "Email or password was invalid",
  detail: "Password is invalid",
})

const Template: Story<AlertBannerProps> = (args) => <AlertBanner {...args} />

export const Warning = Template.bind({})
Warning.args = {
  text: "This is a warning",
  severity: "warning",
}

export const ErrorWithDefaultMessage = Template.bind({})
ErrorWithDefaultMessage.args = {
  text: "This is an error",
  severity: "error",
}

export const ErrorWithErrorMessage = Template.bind({})
ErrorWithErrorMessage.args = {
  error: mockError,
  severity: "error",
}

export const WarningWithDismiss = Template.bind({})
WarningWithDismiss.args = {
  text: "This is a warning",
  dismissible: true,
  severity: "warning",
}

export const ErrorWithDismiss = Template.bind({})
ErrorWithDismiss.args = {
  error: mockError,
  dismissible: true,
  severity: "error",
}

export const WarningWithAction = Template.bind({})
WarningWithAction.args = {
  text: "This is a warning",
  actions: [ExampleAction],
  severity: "warning",
}

export const ErrorWithAction = Template.bind({})
ErrorWithAction.args = {
  error: mockError,
  actions: [ExampleAction],
  severity: "error",
}

export const WarningWithActionAndDismiss = Template.bind({})
WarningWithActionAndDismiss.args = {
  text: "This is a warning",
  actions: [ExampleAction],
  dismissible: true,
  severity: "warning",
}

export const ErrorWithActionAndDismiss = Template.bind({})
ErrorWithActionAndDismiss.args = {
  error: mockError,
  actions: [ExampleAction],
  dismissible: true,
  severity: "error",
}

export const ErrorWithRetry = Template.bind({})
ErrorWithRetry.args = {
  error: mockError,
  retry: () => null,
  dismissible: true,
  severity: "error",
}

export const ErrorWithActionRetryAndDismiss = Template.bind({})
ErrorWithActionRetryAndDismiss.args = {
  error: mockError,
  actions: [ExampleAction],
  retry: () => null,
  dismissible: true,
  severity: "error",
}

export const ErrorAsWarning = Template.bind({})
ErrorAsWarning.args = {
  error: mockError,
  severity: "warning",
}

const WithChildren: Story<AlertBannerProps> = (args) => (
  <AlertBanner {...args}>
    <div>
      This is a message with a <Link href="#">link</Link>
    </div>
  </AlertBanner>
)

export const InfoWithChildContent = WithChildren.bind({})
InfoWithChildContent.args = {
  severity: "info",
}
