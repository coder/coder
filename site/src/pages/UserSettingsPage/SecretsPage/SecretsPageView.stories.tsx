import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import {
	type CreateUserSecretRequest,
	type ImportUserSecretsRequest,
	MaxSecretsFileBytes,
	type UpdateUserSecretRequest,
	type UserSecret,
} from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockImportedUserSecrets,
	MockUserSecrets,
	mockApiError,
} from "#/testHelpers/entities";
import { SAVED_SECRET_VALUE_DISPLAY } from "./SecretDialog";
import { SecretsPageView } from "./SecretsPageView";

const visibleSecrets = MockUserSecrets.slice(0, 4);
const PLACEHOLDER_INPUT = "placeholder input";

const meta: Meta<typeof SecretsPageView> = {
	title: "pages/UserSettingsPage/SecretsPageView",
	component: SecretsPageView,
	// TODO: Stories in this file fail when pixel runs their play functions. Fix them and remove the exclude.
	parameters: { pixel: { exclude: true } },
	args: {
		secrets: visibleSecrets,
		isLoading: false,
		hasLoaded: true,
		isCreating: false,
		isUpdating: false,
		isDeleting: false,
		onCreateSecret: fn(),
		onUpdateSecret: fn(),
		onImportSecrets: fn(),
		onDeleteSecret: fn(),
		onToggleSecretEnabled: fn(),
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
type ToggleSecretEnabledMock = ReturnType<
	typeof fn<(secret: UserSecret, enabled: boolean) => Promise<void> | void>
>;

const waitForDialogToClose = async (body: ReturnType<typeof within>) => {
	await waitFor(() => {
		expect(body.queryByRole("dialog")).not.toBeInTheDocument();
	});
};

const uploadImportFile = async (canvasElement: HTMLElement, file: File) => {
	const user = userEvent.setup({ applyAccept: false });
	const canvas = within(canvasElement);
	const body = within(canvasElement.ownerDocument.body);
	await user.click(canvas.getByRole("button", { name: "Add secret" }));
	const dialog = within(await body.findByRole("dialog"));
	await user.upload(dialog.getByTestId("file-upload"), file);
	return { user, dialog, body };
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
	enabled: request.enabled ?? true,
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

		const docsLink = canvas.getByRole("link", { name: "Read the docs" });
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
			canvas.getByRole("status", { name: "Loading" }),
		).toBeInTheDocument();
		await expect(canvas.queryByText("No secrets yet")).not.toBeInTheDocument();
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
		await waitFor(() =>
			expect(
				dialog.getByText("Saved value will be cleared when you update."),
			).toBeVisible(),
		);
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

const importSecretsSuccess = fn<
	(request: ImportUserSecretsRequest) => Promise<UserSecret[]>
>(async () => MockImportedUserSecrets);

export const ImportSecretsFromFileSubmit: Story = {
	args: {
		onImportSecrets: importSecretsSuccess,
	},
	beforeEach: () => {
		importSecretsSuccess.mockClear();
	},
	play: async ({ canvasElement }) => {
		const { body } = await uploadImportFile(
			canvasElement,
			new File(["A=1\nB=2"], "secrets.ENV", { type: "text/plain" }),
		);

		await waitFor(() => expect(importSecretsSuccess).toHaveBeenCalledTimes(1));
		expect(importSecretsSuccess).toHaveBeenCalledWith({
			format: "env",
			content: "A=1\nB=2",
		});
		await waitForDialogToClose(body);
	},
};

const importSecretsValidationError = fn<
	(request: ImportUserSecretsRequest) => Promise<UserSecret[]>
>(async () => {
	throw mockApiError({
		message: "Validation failed.",
		validations: [
			{
				field: "secrets[1].value",
				detail: "Value is required.",
			},
		],
	});
});

export const ImportSecretsValidationError: Story = {
	args: {
		onImportSecrets: importSecretsValidationError,
	},
	beforeEach: () => {
		importSecretsValidationError.mockClear();
	},
	play: async ({ canvasElement }) => {
		const { dialog } = await uploadImportFile(
			canvasElement,
			new File(["A=1\nB="], "secrets.env", { type: "text/plain" }),
		);

		await waitFor(() =>
			expect(importSecretsValidationError).toHaveBeenCalledTimes(1),
		);
		await waitFor(() =>
			expect(dialog.getByText("secrets[1].value")).toBeVisible(),
		);
		expect(dialog.getByText("Value is required.")).toBeVisible();
		expect(dialog.getByRole("heading", { name: "Add secret" })).toBeVisible();
	},
};

const importSecretsUnsupportedFile = fn<
	(request: ImportUserSecretsRequest) => Promise<UserSecret[]>
>(async () => MockImportedUserSecrets);

export const ImportSecretsUnsupportedFile: Story = {
	args: {
		onImportSecrets: importSecretsUnsupportedFile,
	},
	beforeEach: () => {
		importSecretsUnsupportedFile.mockClear();
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		const dropZone = dialog.getByRole("button", {
			name: /Import secrets from a file/,
		});
		const dataTransfer = new DataTransfer();
		dataTransfer.items.add(
			new File(["not a secret"], "bad.txt", { type: "text/plain" }),
		);
		dropZone.dispatchEvent(
			new DragEvent("drop", {
				bubbles: true,
				cancelable: true,
				dataTransfer,
			}),
		);

		await waitFor(() =>
			expect(
				dialog.getByText(
					"Unsupported file type. Import a .env, .json, .yaml, or .yml file.",
				),
			).toBeVisible(),
		);
		expect(importSecretsUnsupportedFile).not.toHaveBeenCalled();
	},
};

const importSecretsTooLarge = fn<
	(request: ImportUserSecretsRequest) => Promise<UserSecret[]>
>(async () => MockImportedUserSecrets);

export const ImportSecretsTooLarge: Story = {
	args: {
		onImportSecrets: importSecretsTooLarge,
	},
	beforeEach: () => {
		importSecretsTooLarge.mockClear();
	},
	play: async ({ canvasElement }) => {
		const { dialog } = await uploadImportFile(
			canvasElement,
			new File([new Uint8Array(MaxSecretsFileBytes + 1)], "too-large.env"),
		);

		await waitFor(() =>
			expect(
				dialog.getByText(
					"File is too large. Import a file of 1 MiB or smaller.",
				),
			).toBeVisible(),
		);
		expect(importSecretsTooLarge).not.toHaveBeenCalled();
	},
};

const importSecretsParseError = fn<
	(request: ImportUserSecretsRequest) => Promise<UserSecret[]>
>(async () => {
	throw mockApiError({
		message: "Failed to parse secrets file.",
		detail: "Line 2 must contain KEY=VALUE.",
	});
});

export const ImportSecretsParseError: Story = {
	args: {
		onImportSecrets: importSecretsParseError,
	},
	beforeEach: () => {
		importSecretsParseError.mockClear();
	},
	play: async ({ canvasElement }) => {
		const { dialog } = await uploadImportFile(
			canvasElement,
			new File(["GOOD=1\nbad line"], "secrets.env"),
		);

		await waitFor(() =>
			expect(dialog.getByText("Failed to parse secrets file.")).toBeVisible(),
		);
		expect(dialog.getByText("Line 2 must contain KEY=VALUE.")).toBeVisible();
		expect(dialog.queryByText("Response data")).not.toBeInTheDocument();
		expect(dialog.queryByText("Stack Trace")).not.toBeInTheDocument();
	},
};

export const ImportSecretsFileReadError: Story = {
	beforeEach: () => {
		const readAsText = spyOn(FileReader.prototype, "readAsText");
		readAsText.mockImplementation(function (this: FileReader) {
			this.dispatchEvent(new ProgressEvent("error"));
		});
		return () => readAsText.mockRestore();
	},
	play: async ({ canvasElement }) => {
		const { dialog } = await uploadImportFile(
			canvasElement,
			new File(["A=1"], "secrets.env"),
		);

		await waitFor(() =>
			expect(
				dialog.getByText("Failed to read the selected file."),
			).toBeVisible(),
		);
	},
};

const pendingImport = createDeferred<UserSecret[]>();
const importSecretsPending = fn<
	(request: ImportUserSecretsRequest) => Promise<UserSecret[]>
>(() => pendingImport.promise);

export const ImportSecretsPending: Story = {
	args: {
		onImportSecrets: importSecretsPending,
	},
	beforeEach: () => {
		importSecretsPending.mockClear();
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		const input = dialog.getByTestId("file-upload");
		await user.upload(input, new File(["A=1"], "secrets.env"));

		await waitFor(() => expect(importSecretsPending).toHaveBeenCalledTimes(1));
		expect(dialog.getByRole("button", { name: "Cancel" })).toBeDisabled();
		expect(dialog.getByRole("button", { name: "Save" })).toBeDisabled();
		expect(input).toBeDisabled();
		await user.upload(input, new File(["B=2"], "second.env"));
		expect(importSecretsPending).toHaveBeenCalledTimes(1);
	},
};

const importSecretsAfterRemoval = fn<
	(request: ImportUserSecretsRequest) => Promise<UserSecret[]>
>(async () => MockImportedUserSecrets);

export const ImportSecretsRemoveAndRetry: Story = {
	args: {
		onImportSecrets: importSecretsAfterRemoval,
	},
	beforeEach: () => {
		importSecretsAfterRemoval.mockClear();
	},
	play: async ({ canvasElement }) => {
		const { user, dialog, body } = await uploadImportFile(
			canvasElement,
			new File(["not a secret"], "bad.txt"),
		);

		await waitFor(() =>
			expect(
				dialog.getByText(
					"Unsupported file type. Import a .env, .json, .yaml, or .yml file.",
				),
			).toBeVisible(),
		);
		await user.click(dialog.getByRole("button", { name: "Remove file" }));
		expect(
			dialog.queryByText(
				"Unsupported file type. Import a .env, .json, .yaml, or .yml file.",
			),
		).not.toBeInTheDocument();
		await user.upload(
			dialog.getByTestId("file-upload"),
			new File(["A=1"], "secrets.env"),
		);
		await waitFor(() =>
			expect(importSecretsAfterRemoval).toHaveBeenCalledTimes(1),
		);
		await waitForDialogToClose(body);
	},
};

export const ToggleEnabledSubmit: Story = {
	args: {
		onToggleSecretEnabled: fn<
			(secret: UserSecret, enabled: boolean) => Promise<void>
		>(async () => {}),
	},
	play: async ({ canvasElement, args }) => {
		const onToggleSecretEnabled =
			args.onToggleSecretEnabled as ToggleSecretEnabledMock;
		onToggleSecretEnabled.mockClear();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const secret = findVisibleSecretByName("EXAMPLE_TOKEN");

		const toggle = canvas.getByRole("switch", {
			name: `Toggle secret ${secret.name}`,
		});
		await expect(toggle).toBeChecked();
		await user.click(toggle);

		await waitFor(() => expect(onToggleSecretEnabled).toHaveBeenCalledTimes(1));
		expect(onToggleSecretEnabled).toHaveBeenCalledWith(secret, false);
	},
};

export const ToggleEnabledMutationErrorDisplay: Story = {
	args: {
		onToggleSecretEnabled: fn<
			(secret: UserSecret, enabled: boolean) => Promise<void>
		>(async () => {
			throw mockApiError({ message: "Failed to disable secret." });
		}),
	},
	play: async ({ canvasElement, args }) => {
		const onToggleSecretEnabled =
			args.onToggleSecretEnabled as ToggleSecretEnabledMock;
		onToggleSecretEnabled.mockClear();
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const secret = findVisibleSecretByName("EXAMPLE_TOKEN");

		const toggle = canvas.getByRole("switch", {
			name: `Toggle secret ${secret.name}`,
		});
		await user.click(toggle);

		await waitFor(() => expect(onToggleSecretEnabled).toHaveBeenCalledTimes(1));
		// Handler rejected; the parent owns the secret state so the switch
		// remains checked in this story where no state change is applied.
		await expect(toggle).toBeChecked();
	},
};

export const ToggleEnabledDisabledForTargetlessSecret: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const secret = findVisibleSecretByName("SERVICE_PASSWORD");

		const toggle = canvas.getByRole("switch", {
			name: `Toggle secret ${secret.name}`,
		});
		await expect(toggle).not.toBeChecked();
		await expect(toggle).toBeDisabled();
	},
};
