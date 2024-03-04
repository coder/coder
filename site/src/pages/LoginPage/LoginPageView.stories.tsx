import type { Meta, StoryObj } from "@storybook/react";
import {
  MockAuthMethodsAll,
  MockAuthMethodsExternal,
  MockAuthMethodsPasswordOnly,
  mockApiError,
} from "testHelpers/entities";
import { LoginPageView } from "./LoginPageView";

const meta: Meta<typeof LoginPageView> = {
  title: "pages/LoginPage",
  component: LoginPageView,
};

export default meta;
type Story = StoryObj<typeof LoginPageView>;

export const Example: Story = {
  args: {
    authMethods: MockAuthMethodsPasswordOnly,
  },
};

export const WithExternalAuthMethods: Story = {
  args: {
    authMethods: MockAuthMethodsExternal,
  },
};

export const WithAllAuthMethods: Story = {
  args: {
    authMethods: MockAuthMethodsAll,
  },
};

export const AuthError: Story = {
  args: {
    error: mockApiError({
      message: "Incorrect email or password.",
    }),
    authMethods: MockAuthMethodsPasswordOnly,
  },
};

export const ExternalAuthError: Story = {
  args: {
    error: mockApiError({
      message: "Incorrect email or password.",
    }),
    authMethods: MockAuthMethodsAll,
  },
};

export const LoadingAuthMethods: Story = {
  args: {
    authMethods: undefined,
  },
};

export const SigningIn: Story = {
  args: {
    isSigningIn: true,
    authMethods: MockAuthMethodsPasswordOnly,
  },
};
