import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { rolesQueryKey } from "#/api/queries/roles";
import { authMethodsQueryKey } from "#/api/queries/users";
import {
	MockAuthMethodsPasswordOnly,
	MockUserMember,
	mockApiError,
} from "#/testHelpers/entities";
import { withDashboardProvider, withToaster } from "#/testHelpers/storybook";
import CreateUserPage from "./CreateUserPage";

const meta = {
	title: "pages/CreateUserPage/CreateUserPage",
	component: CreateUserPage,
	decorators: [withToaster, withDashboardProvider],
	parameters: {
		queries: [
			{ key: authMethodsQueryKey, data: MockAuthMethodsPasswordOnly },
			{ key: rolesQueryKey, data: [] },
		],
	},
} satisfies Meta<typeof CreateUserPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ShowsSuccessNotificationOnSubmit: Story = {
	beforeEach: () => {
		spyOn(API, "createUser").mockResolvedValue({
			...MockUserMember,
			username: "someuser",
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		await fillForm(canvas, user);
		await within(document.body).findByText(
			'User "someuser" created successfully.',
		);
	},
};

export const ShowsServiceAccountSuccessNotificationOnSubmit: Story = {
	parameters: {
		features: ["service_accounts"],
	},
	beforeEach: () => {
		spyOn(API, "createUser").mockResolvedValue({
			...MockUserMember,
			username: "someuser",
			is_service_account: true,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		await fillForm(canvas, user, "service account");

		await expect(API.createUser).toHaveBeenCalledWith(
			expect.objectContaining({ service_account: true }),
		);
		await within(document.body).findByText(
			'Service account "someuser" created successfully.',
		);
	},
};

export const ShowsErrorWhenUserCreationFails: Story = {
	beforeEach: () => {
		spyOn(API, "createUser").mockRejectedValue(
			mockApiError({ message: "Username already in use." }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		await fillForm(canvas, user);
		await canvas.findAllByText("Username already in use.");
	},
};

export const ShowsErrorWhenServiceAccountCreationFails: Story = {
	parameters: {
		features: ["service_accounts"],
	},
	beforeEach: () => {
		// An API error without a message falls back to our own copy.
		spyOn(API, "createUser").mockRejectedValue(mockApiError({ message: "" }));
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const user = userEvent.setup();
		await fillForm(canvas, user, "service account");
		await within(document.body).findByText(
			'Failed to create service account "someuser".',
		);
	},
};

async function fillForm(
	canvas: ReturnType<typeof within>,
	user: ReturnType<typeof userEvent.setup>,
	loginType: "password" | "service account" = "password",
) {
	const isPasswordLogin = loginType === "password";
	const body = within(document.body);

	await user.type(await canvas.findByLabelText("Username"), "someuser");
	if (isPasswordLogin) {
		await user.type(canvas.getByLabelText(/email/i), "someone@coder.com");
	}

	await user.click(canvas.getByTestId("login-type-input"));
	await user.click(
		await body.findByRole("option", { name: new RegExp(loginType, "i") }),
	);

	if (isPasswordLogin) {
		await user.type(
			await canvas.findByTestId("password-input"),
			"SomeSecurePassword!",
		);
	}

	await user.click(canvas.getByRole("button", { name: /save/i }));
}
