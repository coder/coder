import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
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
		spyOn(API, "createUser").mockResolvedValue(MockUserMember);
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

async function fillForm(
	canvas: ReturnType<typeof within>,
	user: ReturnType<typeof userEvent.setup>,
) {
	await user.type(await canvas.findByLabelText("Username"), "someuser");
	await user.type(canvas.getByLabelText(/email/i), "someone@coder.com");

	const body = within(document.body);
	await user.click(canvas.getByTestId("login-type-input"));
	await user.click(await body.findByRole("option", { name: /password/i }));

	await user.type(
		await canvas.findByTestId("password-input"),
		"SomeSecurePassword!",
	);
	await user.click(canvas.getByRole("button", { name: /save/i }));
}
