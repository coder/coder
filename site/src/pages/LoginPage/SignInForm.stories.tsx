import type { Meta, StoryObj } from "@storybook/react";
import { mockApiError } from "testHelpers/entities";
import { SignInForm } from "./SignInForm";

const meta: Meta<typeof SignInForm> = {
	title: "pages/LoginPage/SignInForm",
	component: SignInForm,
	args: {
		isSigningIn: false,
	},
};

export default meta;
type Story = StoryObj<typeof SignInForm>;

export const SignedOut: Story = {};

export const SigningIn: Story = {
	args: {
		isSigningIn: true,
		authMethods: {
			password: { enabled: true },
			github: { enabled: true, default_provider_configured: false },
			oidc: { enabled: false, signInText: "", iconUrl: "" },
		},
	},
};

export const WithError: Story = {
	args: {
		error: mockApiError({
			message: "Email or password was invalid",
			validations: [
				{
					field: "password",
					detail: "Password is invalid.",
				},
			],
		}),
	},
};

export const WithGithub: Story = {
	args: {
		authMethods: {
			password: { enabled: true },
			github: { enabled: true, default_provider_configured: false },
			oidc: { enabled: false, signInText: "", iconUrl: "" },
		},
	},
};

export const WithOIDC: Story = {
	args: {
		authMethods: {
			password: { enabled: true },
			github: { enabled: false, default_provider_configured: false },
			oidc: { enabled: true, signInText: "", iconUrl: "" },
		},
	},
};

export const WithOIDCWithoutPassword: Story = {
	args: {
		authMethods: {
			password: { enabled: false },
			github: { enabled: false, default_provider_configured: false },
			oidc: { enabled: true, signInText: "", iconUrl: "" },
		},
	},
};

export const WithoutAny: Story = {
	args: {
		authMethods: {
			password: { enabled: false },
			github: { enabled: false, default_provider_configured: false },
			oidc: { enabled: false, signInText: "", iconUrl: "" },
		},
	},
};

export const WithGithubAndOIDC: Story = {
	args: {
		authMethods: {
			password: { enabled: true },
			github: { enabled: true, default_provider_configured: false },
			oidc: { enabled: true, signInText: "", iconUrl: "" },
		},
	},
};
