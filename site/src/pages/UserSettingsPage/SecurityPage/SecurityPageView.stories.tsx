import type { Meta, StoryObj } from "@storybook/react";
import { SecurityPageView } from "./SecurityPage";
import { action } from "@storybook/addon-actions";
import {
  MockAuthMethods,
  MockAuthMethodsWithPasswordType,
} from "testHelpers/entities";
import { ComponentProps } from "react";
import set from "lodash/fp/set";

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
      userLoginType: {
        login_type: "password",
      },
      authMethods: MockAuthMethods,
      closeConfirmation: action("closeConfirmation"),
      confirm: action("confirm"),
      error: undefined,
      isConfirming: false,
      isUpdating: false,
      openConfirmation: action("openConfirmation"),
    },
  },
};

const meta: Meta<typeof SecurityPageView> = {
  title: "pages/SecurityPageView",
  component: SecurityPageView,
  args: defaultArgs,
};

export default meta;
type Story = StoryObj<typeof SecurityPageView>;

export const UsingOIDC: Story = {};

export const NoOIDCAvailable: Story = {
  args: {
    ...defaultArgs,
    oidc: undefined,
  },
};

export const UserLoginTypeIsPassword: Story = {
  args: set(
    "oidc.section.authMethods",
    MockAuthMethodsWithPasswordType,
    defaultArgs,
  ),
};

export const ConfirmingOIDCConversion: Story = {
  args: set(
    "oidc.section",
    {
      ...defaultArgs.oidc?.section,
      authMethods: MockAuthMethodsWithPasswordType,
      isConfirming: true,
    },
    defaultArgs,
  ),
};
