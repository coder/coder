import type { Meta, StoryObj } from "@storybook/react-vite";
import set from "lodash/fp/set";
import { type ComponentProps, type FC, useState } from "react";
import { action } from "storybook/actions";
import { expect, fn, userEvent, within } from "storybook/test";
import {
	MockAuthMethodsAll,
	MockAuthMethodsPasswordOnly,
} from "#/testHelpers/entities";
import { SecurityPageView } from "./SecurityPage";

type SingleSignOnSectionProps = NonNullable<
	ComponentProps<typeof SecurityPageView>["oidc"]
>["section"];

const defaultSingleSignOnSection: SingleSignOnSectionProps = {
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
};

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
		section: defaultSingleSignOnSection,
	},
};

const GitHubToOIDCConversionHarness: FC<
	ComponentProps<typeof SecurityPageView>
> = ({ security, oidc }) => {
	const [isConfirming, setIsConfirming] = useState(false);

	if (!oidc) {
		return <SecurityPageView security={security} />;
	}

	return (
		<SecurityPageView
			security={security}
			oidc={{
				section: {
					...oidc.section,
					closeConfirmation: () => setIsConfirming(false),
					isConfirming,
					openConfirmation: () => setIsConfirming(true),
				},
			}}
		/>
	);
};

const confirmGitHubToOIDCConversion = fn();

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

export const ConfirmingOIDCConversion: Story = {
	args: set(
		"oidc.section",
		{
			...defaultSingleSignOnSection,
			authMethods: MockAuthMethodsAll,
			isConfirming: true,
		},
		defaultArgs,
	),
};

export const AuthenticatedWithGithub: Story = {
	args: {
		...defaultArgs,
		oidc: {
			section: {
				...defaultSingleSignOnSection,
				userLoginType: {
					login_type: "github",
				},
				authMethods: MockAuthMethodsAll,
			},
		},
	},
};

export const AuthenticatedWithOIDC: Story = {
	args: {
		...defaultArgs,
		oidc: {
			section: {
				...defaultSingleSignOnSection,
				userLoginType: {
					login_type: "oidc",
				},
				authMethods: MockAuthMethodsAll,
			},
		},
	},
};

export const ConvertGithubToOIDC: Story = {
	args: {
		...defaultArgs,
		oidc: {
			section: {
				...defaultSingleSignOnSection,
				authMethods: MockAuthMethodsAll,
				confirm: confirmGitHubToOIDCConversion,
				userLoginType: {
					login_type: "github",
				},
			},
		},
	},
	render: (args) => <GitHubToOIDCConversionHarness {...args} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		await user.click(
			await canvas.findByRole("button", { name: /Switch to Google/ }),
		);

		const dialog = await within(document.body).findByRole("dialog", {
			name: "Change login type",
		});
		expect(within(dialog).queryByLabelText("Confirm your password")).toBeNull();
		await user.click(
			await within(dialog).findByRole("button", { name: "Update" }),
		);
		expect(confirmGitHubToOIDCConversion).toHaveBeenCalledWith(undefined);
	},
};
