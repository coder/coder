import {
	MockAuthMethodsAll,
	MockAuthMethodsPasswordOnly,
} from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import set from "lodash/fp/set";
import type { ComponentProps } from "react";
import { action } from "storybook/actions";
import { SecurityPageView } from "./SecurityPage";

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
			authMethods: MockAuthMethodsPasswordOnly,
			closeConfirmation: action("closeConfirmation"),
			confirm: action("confirm"),
			error: null,
			isConfirming: false,
			isUpdating: false,
			openConfirmation: action("openConfirmation"),
		},
	},
};

const meta: Meta<typeof SecurityPageView> = {
	title: "pages/UserSettingsPage/SecurityPageView",
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
	args: set("oidc.section.authMethods", MockAuthMethodsAll, defaultArgs),
};

export const UserLoginTypeIsOIDC: Story = {
	args: set("oidc.section.userLoginType", { login_type: "oidc" }, defaultArgs),
};

export const OIDCAuthenticated: Story = {
	args: set(
		"oidc.section",
		{
			...defaultArgs.oidc?.section,
			authMethods: MockAuthMethodsAll,
			isConfirming: true,
		},
		defaultArgs,
	),
};
