import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import type {
	CreateUserSecretRequest,
	UpdateUserSecretRequest,
	UserSecret,
} from "#/api/typesGenerated";
import { MockUserSecrets, mockApiError } from "#/testHelpers/entities";
import { SecretsPageView } from "./SecretsPageView";

const visibleSecrets = MockUserSecrets.slice(0, 4);
const placeholderInput = "placeholder input";

type CapturedCreateRequest = Omit<CreateUserSecretRequest, "value"> & {
	hasValue: boolean;
};

type CapturedUpdateRequest = Omit<UpdateUserSecretRequest, "value"> & {
	hasValue: boolean;
};

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

const captureCreateRequest = (
	request: CreateUserSecretRequest,
): CapturedCreateRequest => {
	const { value, ...publicRequest } = request;
	return {
		...publicRequest,
		hasValue: value.length > 0,
	};
};

const captureUpdateRequest = (
	request: UpdateUserSecretRequest,
): CapturedUpdateRequest => {
	const { value, ...publicRequest } = request;
	return {
		...publicRequest,
		hasValue: Boolean(value && value.length > 0),
	};
};

const uploadInput = (dialog: HTMLElement): HTMLInputElement => {
	const input = within(dialog).getByTestId("secret-upload-input");
	if (!(input instanceof HTMLInputElement)) {
		throw new Error("Expected secret upload input.");
	}
	return input;
};

export const Loaded: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByRole("table", { name: "User secrets" }),
		).toBeInTheDocument();
		await expect(canvas.getByText("env")).toBeInTheDocument();
		await expect(canvas.getByText("file")).toBeInTheDocument();
		await expect(canvas.getByText("env + file")).toBeInTheDocument();
		await expect(canvas.getByText("not injected")).toBeInTheDocument();
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
};

export const ListLoadError: Story = {
	args: {
		secrets: [],
		hasLoaded: false,
		getSecretsError: mockApiError({
			message: "Failed to load secrets.",
		}),
	},
};

export const AddDialogOpened: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = await body.findByRole("dialog");
		await expect(dialog).toHaveAttribute("data-state", "open");
		await expect(
			within(dialog).getByRole("heading", { name: "Add secret" }),
		).toBeInTheDocument();
		await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

export const AddDialogDuplicateEnvValidationError: Story = {
	args: {
		secrets: MockUserSecrets,
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		await user.type(dialog.getByLabelText("Name"), "duplicate-env");
		await user.type(dialog.getByLabelText("Env var"), "OPENAI_API_KEY");
		await user.tab();

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
		await user.type(dialog.getByLabelText("Name"), "example-secret");
		await user.type(dialog.getByLabelText("Env var"), "EXAMPLE_SECRET");
		await user.type(dialog.getByLabelText("Value"), placeholderInput);

		await expect(dialog.getByRole("button", { name: "Save" })).toBeEnabled();
		await user.click(dialog.getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

const addSaveRequests: CapturedCreateRequest[] = [];

export const AddSecretSubmit: Story = {
	args: {
		onCreateSecret: async (request: CreateUserSecretRequest) => {
			addSaveRequests.push(captureCreateRequest(request));
			return createSecretFromRequest(request);
		},
	},
	play: async ({ canvasElement }) => {
		addSaveRequests.length = 0;
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		await user.type(dialog.getByLabelText("Name"), "example-secret");
		await user.type(dialog.getByLabelText("Env var"), "EXAMPLE_SECRET");
		await user.type(dialog.getByLabelText("File path"), "~/secrets/example");
		await user.type(dialog.getByLabelText("Value"), placeholderInput);
		await user.type(dialog.getByLabelText("Description"), "Example secret");
		await user.click(dialog.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(addSaveRequests).toHaveLength(1));
		expect(addSaveRequests[0]).toEqual({
			name: "example-secret",
			env_name: "EXAMPLE_SECRET",
			file_path: "~/secrets/example",
			description: "Example secret",
			hasValue: true,
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
		const secret = visibleSecrets[0] as UserSecret;

		await user.click(
			canvas.getByRole("button", {
				name: `Open secret actions for ${secret.name}`,
			}),
		);
		await user.click(
			await body.findByRole("menuitem", { name: "Edit secret..." }),
		);

		const dialog = await body.findByRole("dialog");
		await expect(dialog).toHaveAttribute("data-state", "open");
		await expect(
			within(dialog).getByRole("heading", { name: "Edit secret" }),
		).toBeInTheDocument();
		await expect(within(dialog).getByLabelText("Value")).toHaveValue("");
		await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

const updateRequests: Array<{
	name: string;
	request: CapturedUpdateRequest;
}> = [];

export const EditSecretSubmit: Story = {
	args: {
		onUpdateSecret: async (name: string, request: UpdateUserSecretRequest) => {
			updateRequests.push({
				name,
				request: captureUpdateRequest(request),
			});
			return visibleSecrets.find((secret) => secret.name === name);
		},
	},
	play: async ({ canvasElement }) => {
		updateRequests.length = 0;
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
			await body.findByRole("menuitem", { name: "Edit secret..." }),
		);

		const dialog = within(await body.findByRole("dialog"));
		const description = dialog.getByLabelText("Description");
		await user.clear(description);
		await user.type(description, "Updated example description");
		await user.click(dialog.getByRole("button", { name: "Update" }));

		await waitFor(() => expect(updateRequests).toHaveLength(1));
		expect(updateRequests[0]).toEqual({
			name: secret.name,
			request: {
				description: "Updated example description",
				hasValue: false,
			},
		});
		await waitForDialogToClose(body);
		expectNoValueField(body);
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
			await body.findByRole("menuitem", { name: "Edit secret..." }),
		).toBeInTheDocument();
		await user.click(body.getByRole("menuitem", { name: "Delete..." }));

		const dialog = await body.findByRole("dialog");
		await expect(dialog).toHaveAttribute("data-state", "open");
		await expect(
			within(dialog).getByRole("heading", { name: "Delete secret" }),
		).toBeInTheDocument();
		await user.click(within(dialog).getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

const deletedSecrets: UserSecret[] = [];

export const DeleteConfirmSubmit: Story = {
	args: {
		onDeleteSecret: (secret: UserSecret) => {
			deletedSecrets.push(secret);
		},
	},
	play: async ({ canvasElement }) => {
		deletedSecrets.length = 0;
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const secret = visibleSecrets[0] as UserSecret;

		await user.click(
			canvas.getByRole("button", {
				name: `Open secret actions for ${secret.name}`,
			}),
		);
		await user.click(await body.findByRole("menuitem", { name: "Delete..." }));
		await user.click(await body.findByRole("button", { name: "Delete" }));

		await waitFor(() => expect(deletedSecrets).toEqual([secret]));
		await waitForDialogToClose(body);
	},
};

export const DeleteAndCancel: Story = {
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
		await user.click(await body.findByRole("menuitem", { name: "Delete..." }));
		await user.click(await body.findByRole("button", { name: "Cancel" }));

		await waitForDialogToClose(body);
	},
};

const envUploadRequests: CapturedCreateRequest[] = [];

export const EnvUploadImportSubmit: Story = {
	args: {
		onCreateSecret: async (request: CreateUserSecretRequest) => {
			envUploadRequests.push(captureCreateRequest(request));
			return createSecretFromRequest(request);
		},
	},
	play: async ({ canvasElement }) => {
		envUploadRequests.length = 0;
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = await body.findByRole("dialog");
		await user.upload(
			uploadInput(dialog),
			new File([`EXAMPLE_TOKEN=${placeholderInput}`], "secrets.env", {
				type: "text/plain",
			}),
		);

		await waitFor(() => expect(envUploadRequests).toHaveLength(1));
		expect(envUploadRequests[0]).toEqual({
			name: "EXAMPLE_TOKEN",
			env_name: "EXAMPLE_TOKEN",
			hasValue: true,
		});
		await waitForDialogToClose(body);
		expectNoValueField(body);
	},
};

const jsonUploadRequests: CapturedCreateRequest[] = [];

export const JsonUploadImportSubmit: Story = {
	args: {
		onCreateSecret: async (request: CreateUserSecretRequest) => {
			jsonUploadRequests.push(captureCreateRequest(request));
			return createSecretFromRequest(request);
		},
	},
	play: async ({ canvasElement }) => {
		jsonUploadRequests.length = 0;
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = await body.findByRole("dialog");
		await user.upload(
			uploadInput(dialog),
			new File(
				[
					JSON.stringify([
						{
							name: "json-secret",
							env_name: "JSON_SECRET",
							description: "Imported from JSON",
							value: placeholderInput,
						},
					]),
				],
				"secrets.json",
				{ type: "application/json" },
			),
		);

		await waitFor(() => expect(jsonUploadRequests).toHaveLength(1));
		expect(jsonUploadRequests[0]).toEqual({
			name: "json-secret",
			env_name: "JSON_SECRET",
			description: "Imported from JSON",
			hasValue: true,
		});
		await waitForDialogToClose(body);
		expectNoValueField(body);
	},
};

export const UploadControlKeyboardOperable: Story = {
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		const input = uploadInput(await body.findByRole("dialog"));
		const inputClick = fn();
		input.click = () => {
			inputClick();
		};

		const uploadButton = dialog.getByRole("button", { name: "Upload" });
		uploadButton.focus();
		await expect(uploadButton).toHaveFocus();
		await user.keyboard("{Enter}");

		await waitFor(() => expect(inputClick).toHaveBeenCalledTimes(1));
		await user.click(dialog.getByRole("button", { name: "Cancel" }));
		await waitForDialogToClose(body);
	},
};

export const CreateMutationErrorDisplay: Story = {
	args: {
		onCreateSecret: async () => {
			throw {
				isAxiosError: true,
				status: 409,
				response: {
					status: 409,
					data: {
						message:
							"A secret with that name, environment variable, or file path already exists.",
					},
				},
			};
		},
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		await user.click(canvas.getByRole("button", { name: "Add secret" }));
		const dialog = within(await body.findByRole("dialog"));
		await user.type(dialog.getByLabelText("Name"), "conflict-secret");
		await user.type(dialog.getByLabelText("Value"), placeholderInput);
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
