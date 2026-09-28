import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
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

export const Loading: Story = {
	parameters: {
		queries: [{ key: rolesQueryKey, data: [] }],
	},
	beforeEach: () => {
		spyOn(API, "getAuthMethods").mockReturnValue(new Promise(() => {}));
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("status", { name: /loading/i }),
		).toBeVisible();
	},
};

export const AuthMethodsError: Story = {
	parameters: {
		queries: [{ key: rolesQueryKey, data: [] }],
	},
	beforeEach: () => {
		spyOn(API, "getAuthMethods").mockRejectedValue(
			mockApiError({ message: "Failed to load authentication methods." }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("Failed to load authentication methods."),
		).toBeVisible();
	},
};

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

	await user.type(await canvas.findByLabelText(/username/i), "someuser");
	if (isPasswordLogin) {
		await user.type(canvas.getByLabelText(/email/i), "someone@coder.com");
		await user.type(
			canvas.getByTestId("password-input"),
			"SomeSecurePassword!",
		);
	} else {
		const body = within(document.body);
		await user.click(canvas.getByTestId("login-type-input"));
		await user.click(
			await body.findByRole("option", { name: /service account/i }),
		);
		// Wait for the select to finish closing; while it is open or exit-animating
		// Radix locks body pointer-events, which would fail the next interaction.
		await waitFor(() =>
			expect(body.queryByRole("listbox")).not.toBeInTheDocument(),
		);
	}

	await user.click(await canvas.findByRole("button", { name: /save/i }));
}
