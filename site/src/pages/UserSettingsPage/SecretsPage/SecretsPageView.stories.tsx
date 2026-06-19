import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type {
	CreateUserSecretRequest,
	UpdateUserSecretRequest,
	UserSecret,
} from "#/api/typesGenerated";
import { MockUserSecrets, mockApiError } from "#/testHelpers/entities";
import { SAVED_SECRET_VALUE_DISPLAY } from "./SecretDialog";
import { SecretsPageView } from "./SecretsPageView";

const visibleSecrets = MockUserSecrets.slice(0, 4);
const PLACEHOLDER_INPUT = "placeholder input";

const meta: Meta<typeof SecretsPageView> = {
	title: "pages/UserSettingsPage/SecretsPageView",
	component: SecretsPageView,
	args: {
		secrets: visibleSecrets,
		isLoading: false,
		hasLoaded: true,
		isRefreshing: false,
		isCreating: false,
		isUpdating: false,
		isDeleting: false,
		onRefresh: fn(),
		onCreateSecret: fn(),
		onUpdateSecret: fn(),
		onDeleteSecret: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof SecretsPageView>;
type CreateSecretMock = ReturnType<
	typeof fn<(request: CreateUserSecretRequest) => Promise<UserSecret>>
>;
type UpdateSecretMock = ReturnType<
	typeof fn<
		(name: string, request: UpdateUserSecretRequest) => Promise<UserSecret>
	>
>;
type DeleteSecretMock = ReturnType<
	typeof fn<(secret: UserSecret) => Promise<void> | void>
>;

const waitForDialogToClose = async (body: ReturnType<typeof within>) => {
	await waitFor(() => {
		expect(body.queryByRole("dialog")).not.toBeInTheDocument();
	});
};

const expectNoValueField = (body: ReturnType<typeof within>) => {
	expect(body.queryByLabelText("Value")).not.toBeInTheDocument();
};

const createSecretFromRequest = (
	request: CreateUserSecretRequest,
): UserSecret => ({
	id: `created-${request.name}`,
	name: request.name,
	description: request.description ?? "",
	env_name: request.env_name ?? "",
	file_path: request.file_path ?? "",
	created_at: "2026-05-04T00:00:00Z",
	updated_at: "2026-05-04T00:00:00Z",
});

const findVisibleSecretByName = (name: string): UserSecret => {
	const secret = visibleSecrets.find((secret) => secret.name === name);
	if (!secret) {
		throw new Error(`No visible secret named ${name}`);
	}
	return secret;
};

export const Loaded: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("table", { name: "User secrets" }),
		).toBeInTheDocument();
		await expect(canvas.getByText("env var")).toBeInTheDocument();
		await expect(canvas.getByText("file")).toBeInTheDocument();
		await expect(canvas.getByText("env var + file")).toBeInTheDocument();
		await expect(canvas.getByText("not injected")).toBeInTheDocument();

		const docsLink = canvas.getByRole("link", { name: "View docs" });
		await expect(docsLink).toHaveAttribute(
			"href",
			expect.stringContaining("/user-guides/user-secrets"),
		);
	},
};

export const Empty: Story = {
	args: {
		secrets: [],
	},
};

export const Loading: Story = {
	args: {
		secrets: [],
		isLoading: true,
		hasLoaded: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("button", { name: /Refresh/ }),
		).toBeDisabled();
	},
};

export const RefreshingWithRows: Story = {
	args: {
		secrets: visibleSecrets,
		isLoading: false,
		hasLoaded: true,
		isRefreshing: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getAllByText(visibleSecrets[0].name)[0]).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: /Refresh/ }),
		).toBeDisabled();
	},
};

export const ListLoadError: Story = {
	args: {
		secrets: [],
		hasLoaded: true,
		getSecretsError: mockApiError({
			message: "Failed to load secrets.",
		}),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByText("Failed to load secrets.")).toBeVisible();
		await expect(canvas.queryByText("No secrets yet")).not.toBeInTheDocument();
	},
};

export const AddDialogOpened: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = await body.findByRole("dialog");
		const dialogView = within(dialog);
		await expect(
			dialogView.getByRole("heading", { name: "Add secret" }),
		).toBeInTheDocument();
		await expect(dialogView.getByLabelText("Name")).toBeRequired();
		await expect(dialogView.getByLabelText("Name")).toHaveAttribute(
			"placeholder",
			"Secret name",
		);
		await expect(dialogView.getByLabelText("Value")).toBeRequired();
		await expect(dialogView.getByLabelText("Value")).toHaveAttribute(
			"placeholder",
			"Enter secret value",
		);
	},
};

export const AddDialogDuplicateEnvValidationError: Story = {
	args: {
		onCreateSecret: async () => {
			throw mockApiError({
				message: "Validation failed.",
				validations: [
					{
						field: "env_name",
						detail: "Variable already in use. Edit existing variable.",
					},
				],
			});
		},
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		await user.type(dialog.getByLabelText("Name"), "duplicate-env");
		await user.type(
			dialog.getByLabelText("Environment variable"),
			"SERVICE_API_KEY",
		);
		await user.type(dialog.getByLabelText("Value"), PLACEHOLDER_INPUT);
		const saveButton = dialog.getByRole("button", { name: "Save" });
		await waitFor(() => expect(saveButton).toBeEnabled());
		await user.click(saveButton);

		await expect(
			await dialog.findByText(
				"Variable already in use. Edit existing variable.",
			),
		).toBeVisible();
		await user.click(dialog.getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

export const AddSecretFormSaveEnabled: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		const saveButton = dialog.getByRole("button", { name: "Save" });
		await user.type(dialog.getByLabelText("Name"), "example-secret");
		await expect(saveButton).toBeDisabled();
		await user.type(
			dialog.getByLabelText("Environment variable"),
			"EXAMPLE_SECRET",
		);
		await user.type(dialog.getByLabelText("Value"), PLACEHOLDER_INPUT);

		await expect(saveButton).toBeEnabled();
		await user.click(dialog.getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

export const AddSecretSubmit: Story = {
	args: {
		onCreateSecret: fn<
			(request: CreateUserSecretRequest) => Promise<UserSecret>
		>(async (request) => createSecretFromRequest(request)),
	},
	play: async ({ canvasElement, args }) => {
		const onCreateSecret = args.onCreateSecret as CreateSecretMock;
		onCreateSecret.mockClear();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		await user.type(dialog.getByLabelText("Name"), "example-secret");
		await user.type(
			dialog.getByLabelText("Environment variable"),
			"EXAMPLE_SECRET",
		);
		await user.type(dialog.getByLabelText("File path"), "~/secrets/example");
		await user.type(dialog.getByLabelText("Value"), PLACEHOLDER_INPUT);
		await user.type(dialog.getByLabelText("Description"), "Example secret");
		await user.click(dialog.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(onCreateSecret).toHaveBeenCalledTimes(1));
		expect(onCreateSecret).toHaveBeenCalledWith({
			name: "example-secret",
			env_name: "EXAMPLE_SECRET",
			file_path: "~/secrets/example",
			value: PLACEHOLDER_INPUT,
			description: "Example secret",
		});
		await waitForDialogToClose(body);
		expectNoValueField(body);
	},
};

export const EditDialogOpened: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const secret = visibleSecrets[2] as UserSecret;

		await user.click(
			canvas.getByRole("button", {
				name: `Open secret actions for ${secret.name}`,
			}),
		);
		await user.click(
			await body.findByRole("menuitem", { name: "Edit secret" }),
		);

		const dialog = await body.findByRole("dialog");
		const dialogView = within(dialog);
		await expect(
			dialogView.getByRole("heading", { name: "Edit secret" }),
		).toBeInTheDocument();
		await expect(dialogView.getByLabelText("Name")).toHaveValue(secret.name);
		await expect(dialogView.getByLabelText("Name")).toBeDisabled();
		await expect(
			dialogView.getByText("Unique identifier (can’t be changed)."),
		).toBeInTheDocument();
		await expect(dialogView.getByLabelText("Description")).toHaveValue(
			secret.description,
		);
		await expect(dialogView.getByLabelText("Environment variable")).toHaveValue(
			secret.env_name,
		);
		await expect(dialogView.getByLabelText("File path")).toHaveValue(
			secret.file_path,
		);
		const valueField = dialogView.getByLabelText("Value");
		await expect(valueField).toHaveValue(SAVED_SECRET_VALUE_DISPLAY);
		const clearButton = dialogView.getByRole("button", { name: "Clear" });
		await waitFor(() => expect(clearButton).toBeVisible());
		await user.click(valueField);
		await expect(valueField).toHaveValue("");
		await user.tab();
		await expect(valueField).toHaveValue(SAVED_SECRET_VALUE_DISPLAY);
		await expect(
			dialogView.getByRole("button", { name: "Update" }),
		).toBeDisabled();
	},
};

export const EditSecretSubmit: Story = {
	args: {
		onUpdateSecret: fn<
			(name: string, request: UpdateUserSecretRequest) => Promise<UserSecret>
		>(async (name) => findVisibleSecretByName(name)),
	},
	play: async ({ canvasElement, args }) => {
		const onUpdateSecret = args.onUpdateSecret as UpdateSecretMock;
		onUpdateSecret.mockClear();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const secret = visibleSecrets[0] as UserSecret;

		await user.click(
			canvas.getByRole("button", {
				name: `Open secret actions for ${secret.name}`,
			}),
		);
		await user.click(
			await body.findByRole("menuitem", { name: "Edit secret" }),
		);

		const dialog = within(await body.findByRole("dialog"));
		const description = dialog.getByLabelText("Description");
		await user.clear(description);
		await user.type(description, "Updated example description");
		await user.click(dialog.getByRole("button", { name: "Update" }));

		await waitFor(() => expect(onUpdateSecret).toHaveBeenCalledTimes(1));
		expect(onUpdateSecret).toHaveBeenCalledWith(secret.name, {
			description: "Updated example description",
		});
		await waitForDialogToClose(body);
		expectNoValueField(body);
	},
};

export const EditSecretClearValue: Story = {
	args: {
		onUpdateSecret: fn<
			(name: string, request: UpdateUserSecretRequest) => Promise<UserSecret>
		>(async (name) => findVisibleSecretByName(name)),
	},
	play: async ({ canvasElement, args }) => {
		const onUpdateSecret = args.onUpdateSecret as UpdateSecretMock;
		onUpdateSecret.mockClear();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const secret = visibleSecrets[0] as UserSecret;

		await user.click(
			canvas.getByRole("button", {
				name: `Open secret actions for ${secret.name}`,
			}),
		);
		await user.click(
			await body.findByRole("menuitem", { name: "Edit secret" }),
		);

		const dialog = within(await body.findByRole("dialog"));
		const valueField = dialog.getByLabelText("Value");
		const updateButton = dialog.getByRole("button", { name: "Update" });
		await expect(updateButton).toBeDisabled();

		await user.click(dialog.getByRole("button", { name: "Clear" }));
		await expect(valueField).toHaveValue("");
		await expect(valueField).toBeDisabled();
		await expect(
			dialog.getByText("Saved value will be cleared when you update."),
		).toBeVisible();
		await expect(updateButton).toBeEnabled();

		await user.click(dialog.getByRole("button", { name: "Undo" }));
		await expect(valueField).toHaveValue(SAVED_SECRET_VALUE_DISPLAY);
		await expect(valueField).toBeEnabled();
		await expect(updateButton).toBeDisabled();

		await user.click(dialog.getByRole("button", { name: "Clear" }));
		await user.click(updateButton);

		await waitFor(() => expect(onUpdateSecret).toHaveBeenCalledTimes(1));
		expect(onUpdateSecret).toHaveBeenCalledWith(secret.name, { value: "" });
		await waitForDialogToClose(body);
		expectNoValueField(body);
	},
};

export const EditSecretMutationErrorDisplay: Story = {
	args: {
		onUpdateSecret: fn<
			(name: string, request: UpdateUserSecretRequest) => Promise<UserSecret>
		>(async () => {
			throw mockApiError({ message: "Failed to update secret." });
		}),
	},
	play: async ({ canvasElement, args }) => {
		const onUpdateSecret = args.onUpdateSecret as UpdateSecretMock;
		onUpdateSecret.mockClear();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const secret = visibleSecrets[0] as UserSecret;

		await user.click(
			canvas.getByRole("button", {
				name: `Open secret actions for ${secret.name}`,
			}),
		);
		await user.click(
			await body.findByRole("menuitem", { name: "Edit secret" }),
		);

		const dialog = within(await body.findByRole("dialog"));
		const description = dialog.getByLabelText("Description");
		const value = dialog.getByLabelText("Value");
		await user.clear(description);
		await user.type(description, "Updated example description");
		await user.type(value, PLACEHOLDER_INPUT);
		await user.click(dialog.getByRole("button", { name: "Update" }));

		await waitFor(() => expect(onUpdateSecret).toHaveBeenCalledTimes(1));
		await expect(
			await dialog.findByText("Failed to update secret."),
		).toBeVisible();
		await expect(
			dialog.getByRole("heading", { name: "Edit secret" }),
		).toBeVisible();
		await expect(description).toHaveValue("Updated example description");
		await expect(value).toHaveValue(PLACEHOLDER_INPUT);
		await user.click(dialog.getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

export const KebabActionsAndDeleteConfirmation: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const secret = visibleSecrets[0] as UserSecret;

		await user.click(
			canvas.getByRole("button", {
				name: `Open secret actions for ${secret.name}`,
			}),
		);
		await expect(
			await body.findByRole("menuitem", { name: "Edit secret" }),
		).toBeInTheDocument();
		await user.click(body.getByRole("menuitem", { name: "Delete" }));

		const dialog = await body.findByRole("dialog");
		const dialogView = within(dialog);
		await expect(
			dialogView.getByRole("heading", { name: "Delete secret" }),
		).toBeInTheDocument();
		await user.click(dialogView.getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

export const DeleteConfirmSubmit: Story = {
	args: {
		onDeleteSecret: fn<(secret: UserSecret) => void>(),
	},
	play: async ({ canvasElement, args }) => {
		const onDeleteSecret = args.onDeleteSecret as DeleteSecretMock;
		onDeleteSecret.mockClear();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const secret = visibleSecrets[0] as UserSecret;

		await user.click(
			canvas.getByRole("button", {
				name: `Open secret actions for ${secret.name}`,
			}),
		);
		await user.click(await body.findByRole("menuitem", { name: "Delete" }));
		await user.click(await body.findByRole("button", { name: "Delete" }));

		await waitFor(() => expect(onDeleteSecret).toHaveBeenCalledTimes(1));
		expect(onDeleteSecret).toHaveBeenCalledWith(secret);
		await waitForDialogToClose(body);
	},
};

export const DeleteSecretMutationErrorDisplay: Story = {
	args: {
		onDeleteSecret: fn<(secret: UserSecret) => Promise<void>>(async () => {
			throw mockApiError({ message: "Failed to delete secret." });
		}),
	},
	play: async ({ canvasElement, args }) => {
		const onDeleteSecret = args.onDeleteSecret as DeleteSecretMock;
		onDeleteSecret.mockClear();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const secret = visibleSecrets[0] as UserSecret;

		await user.click(
			canvas.getByRole("button", {
				name: `Open secret actions for ${secret.name}`,
			}),
		);
		await user.click(await body.findByRole("menuitem", { name: "Delete" }));
		const dialog = within(await body.findByRole("dialog"));
		await user.click(dialog.getByRole("button", { name: "Delete" }));

		await waitFor(() => expect(onDeleteSecret).toHaveBeenCalledTimes(1));
		await expect(
			dialog.getByRole("heading", { name: "Delete secret" }),
		).toBeVisible();
		await expect(dialog.getByText(secret.name)).toBeVisible();
		await user.click(dialog.getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

export const DeleteAndCancel: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const secret = visibleSecrets[0] as UserSecret;
		const trigger = canvas.getByRole("button", {
			name: `Open secret actions for ${secret.name}`,
		});

		await user.click(trigger);
		await user.click(await body.findByRole("menuitem", { name: "Delete" }));
		await user.click(await body.findByRole("button", { name: "Cancel" }));

		await waitForDialogToClose(body);
		await waitFor(() => expect(trigger).toHaveFocus());
	},
};

export const CreateMutationErrorDisplay: Story = {
	args: {
		onCreateSecret: async () => {
			throw mockApiError({
				message:
					"A secret with that name, environment variable, or file path already exists.",
			});
		},
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		await user.type(dialog.getByLabelText("Name"), "conflict-secret");
		await user.type(dialog.getByLabelText("Value"), PLACEHOLDER_INPUT);
		await user.click(dialog.getByRole("button", { name: "Save" }));

		await expect(
			await dialog.findByText(
				"A secret with that name, environment variable, or file path already exists.",
			),
		).toBeVisible();
		await user.click(dialog.getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
		expectNoValueField(body);
	},
};
