import type { Meta, StoryObj } from "@storybook/react"
import { SecurityPageView } from "./SecurityPage"
import { action } from "@storybook/addon-actions"
import { MockAuthMethods } from "testHelpers/entities"
import { ComponentProps } from "react"
import set from "lodash/fp/set"
import { AuthMethods } from "api/typesGenerated"

const defaultArgs: ComponentProps<typeof SecurityPageView> = {
  security: {
    form: {
      disabled: false,
      error: undefined,
      isLoading: false,
      onSubmit: action("onSubmit"),
    },
  },
  oidc: {
    section: {
      authMethods: MockAuthMethods,
      closeConfirmation: action("closeConfirmation"),
      confirm: action("confirm"),
      error: undefined,
      isConfirming: false,
      isUpdating: false,
      openConfirmation: action("openConfirmation"),
    },
  },
}

const meta: Meta<typeof SecurityPageView> = {
  title: "pages/SecurityPageView",
  component: SecurityPageView,
  args: defaultArgs,
}

export default meta
type Story = StoryObj<typeof SecurityPageView>

export const UsingOIDC: Story = {}

export const NoOIDCAvailable: Story = {
  args: {
    ...defaultArgs,
    oidc: undefined,
  },
}

const authMethodsWithPassword: AuthMethods = {
  ...MockAuthMethods,
  me_login_type: "password",
  github: { enabled: true },
  oidc: { enabled: true, signInText: "", iconUrl: "" },
}

export const UserLoginTypeIsPassword: Story = {
  args: set("oidc.section.authMethods", authMethodsWithPassword, defaultArgs),
}

export const ConfirmingOIDCConversion: Story = {
  args: set(
    "oidc.section",
    {
      ...defaultArgs.oidc?.section,
      authMethods: authMethodsWithPassword,
      isConfirming: true,
    },
    defaultArgs,
  ),
}
