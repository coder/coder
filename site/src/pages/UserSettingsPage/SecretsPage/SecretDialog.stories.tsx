import type { Meta, StoryObj } from "@storybook/react-vite";
import { AxiosError, AxiosHeaders, type AxiosResponse } from "axios";
import { expect, fn, waitFor, within } from "storybook/test";
import type { ApiErrorResponse } from "#/api/errors";
import type {
	ImportUserSecretsRequest,
	UserSecret,
} from "#/api/typesGenerated";
import {
	MockImportedUserSecrets,
	MockUserSecrets,
} from "#/testHelpers/entities";
import { SecretDialog, UNSUPPORTED_SECRETS_FILE_MESSAGE } from "./SecretDialog";

const onImportSecrets = fn<
	(request: ImportUserSecretsRequest) => Promise<UserSecret[]>
>(async () => MockImportedUserSecrets);

const meta: Meta<typeof SecretDialog> = {
	title: "pages/UserSettingsPage/SecretDialog",
	component: SecretDialog,
	args: {
		open: true,
		isSubmitting: false,
		onClose: fn(),
		onCreateSecret: fn(),
		onUpdateSecret: fn(),
		onImportSecrets,
	},
};

export default meta;
type Story = StoryObj<typeof SecretDialog>;

const findDialog = async (
	canvasElement: HTMLElement,
): Promise<ReturnType<typeof within>> => {
	const body = within(canvasElement.ownerDocument.body);
	return within(await body.findByRole("dialog"));
};

const getDropZone = (dialog: ReturnType<typeof within>): HTMLElement =>
	dialog.getByRole("button", { name: /Import secrets from a file/ });

const dropFile = (dropZone: HTMLElement, file: File): void => {
	const dataTransfer = new DataTransfer();
	dataTransfer.items.add(file);
	dropZone.dispatchEvent(
		new DragEvent("dragover", {
			bubbles: true,
			cancelable: true,
			dataTransfer,
		}),
	);
	dropZone.dispatchEvent(
		new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer }),
	);
};

export const DropUnsupportedFile: Story = {
	play: async ({ canvasElement }) => {
		onImportSecrets.mockClear();
		const dialog = await findDialog(canvasElement);

		dropFile(
			getDropZone(dialog),
			new File(["not a secret"], "bad.txt", { type: "text/plain" }),
		);

		await waitFor(() =>
			expect(dialog.getByText(UNSUPPORTED_SECRETS_FILE_MESSAGE)).toBeVisible(),
		);
		expect(onImportSecrets).not.toHaveBeenCalled();
		expect(dialog.getByText("bad.txt")).toBeVisible();
	},
};

export const DropSupportedFile: Story = {
	play: async ({ canvasElement }) => {
		onImportSecrets.mockClear();
		const dialog = await findDialog(canvasElement);

		dropFile(
			getDropZone(dialog),
			new File(["A=1\nB=2"], "secrets.env", { type: "text/plain" }),
		);

		await waitFor(() => expect(onImportSecrets).toHaveBeenCalledTimes(1));
		expect(onImportSecrets).toHaveBeenCalledWith({
			format: "env",
			content: "A=1\nB=2",
		});
		expect(dialog.queryByText(UNSUPPORTED_SECRETS_FILE_MESSAGE)).toBeNull();
	},
};

export const DropZoneDragActiveState: Story = {
	play: async ({ canvasElement }) => {
		const dialog = await findDialog(canvasElement);
		const dropZone = getDropZone(dialog);
		await waitFor(() =>
			expect(dropZone).toHaveAttribute("data-drag-active", "false"),
		);

		const dataTransfer = new DataTransfer();
		dataTransfer.items.add(new File(["A=1"], "secrets.env"));
		dropZone.dispatchEvent(
			new DragEvent("dragover", {
				bubbles: true,
				cancelable: true,
				dataTransfer,
			}),
		);
		await waitFor(() =>
			expect(dropZone).toHaveAttribute("data-drag-active", "true"),
		);

		dropZone.dispatchEvent(
			new DragEvent("dragleave", {
				bubbles: true,
				cancelable: true,
				dataTransfer,
				relatedTarget: canvasElement.ownerDocument.body,
			}),
		);
		await waitFor(() =>
			expect(dropZone).toHaveAttribute("data-drag-active", "false"),
		);
	},
};

const importErrorResponse = (
	status: number,
	data: ApiErrorResponse,
): AxiosResponse<ApiErrorResponse> => ({
	data,
	status,
	statusText: "Bad Request",
	headers: new AxiosHeaders(),
	config: { headers: new AxiosHeaders() },
});

const importRequestError = (
	status: number,
	data: ApiErrorResponse,
): AxiosError<ApiErrorResponse> =>
	new AxiosError(
		`Request failed with status code ${status}`,
		"ERR_BAD_REQUEST",
		undefined,
		undefined,
		importErrorResponse(status, data),
	);

const rejectImportWith = (error: unknown) =>
	fn<(request: ImportUserSecretsRequest) => Promise<UserSecret[]>>(async () => {
		throw error;
	});

const fiftyOneKeyEnvFile = Array.from(
	{ length: 51 },
	(_, index) => `KEY_${index}=value`,
).join("\n");

const tooManySecretsDetail =
	"secrets file contains 51 secrets, which exceeds the maximum of 50 secrets per user";

// ErrorAlert renders its debug payloads behind <summary>Response data</summary>
// and <summary>Stack Trace</summary>, so asserting those labels are absent
// covers both leaks.
export const ImportParseErrorHidesDebugDetail: Story = {
	args: {
		onImportSecrets: rejectImportWith(
			importRequestError(400, {
				message: "Failed to parse secrets file.",
				detail: tooManySecretsDetail,
			}),
		),
	},
	play: async ({ canvasElement }) => {
		const dialog = await findDialog(canvasElement);

		dropFile(
			getDropZone(dialog),
			new File([fiftyOneKeyEnvFile], "fifty_one.env", { type: "text/plain" }),
		);

		await waitFor(() =>
			expect(dialog.getByText("Failed to parse secrets file.")).toBeVisible(),
		);
		expect(dialog.getByText(tooManySecretsDetail)).toBeVisible();
		expect(dialog.queryAllByText(/AxiosError/)).toHaveLength(0);
		expect(dialog.queryAllByText(/"message":/)).toHaveLength(0);
		expect(dialog.queryByText("Response data")).toBeNull();
		expect(dialog.queryByText("Stack Trace")).toBeNull();
	},
};

export const ImportPerEntryErrorNamesKeyAndLine: Story = {
	args: {
		onImportSecrets: rejectImportWith(
			importRequestError(400, {
				message: "Validation failed.",
				validations: [
					{
						field: "secrets[1].name",
						detail:
							'Secret "bad/name" on line 2: Name must not contain /, ?, or #.',
					},
					{
						field: "secrets[2].value",
						detail: 'Secret "EMPTY" on line 3: Value is required.',
					},
				],
			}),
		),
	},
	play: async ({ canvasElement }) => {
		const dialog = await findDialog(canvasElement);

		dropFile(
			getDropZone(dialog),
			new File(["GOOD=1\nbad/name=2\nEMPTY="], "errors.env", {
				type: "text/plain",
			}),
		);

		await waitFor(() =>
			expect(dialog.getByText("Validation failed.")).toBeVisible(),
		);
		expect(dialog.getByText("secrets[1].name")).toBeVisible();
		expect(
			dialog.getByText(
				'Secret "bad/name" on line 2: Name must not contain /, ?, or #.',
			),
		).toBeVisible();
		expect(dialog.getByText("secrets[2].value")).toBeVisible();
		expect(
			dialog.getByText('Secret "EMPTY" on line 3: Value is required.'),
		).toBeVisible();
		expect(dialog.queryByText("Response data")).toBeNull();
		expect(dialog.queryByText("Stack Trace")).toBeNull();
	},
};

// JSON carries no line information, so the backend omits the line clause
// entirely. Guard against a "line 0" placeholder or an empty parenthetical
// reappearing in the rendered entry.
export const ImportPerEntryErrorOmitsLineForJson: Story = {
	args: {
		onImportSecrets: rejectImportWith(
			importRequestError(400, {
				message: "Validation failed.",
				validations: [
					{
						field: "secrets[1].name",
						detail: 'Secret "bad/name": Name must not contain /, ?, or #.',
					},
				],
			}),
		),
	},
	play: async ({ canvasElement }) => {
		const dialog = await findDialog(canvasElement);

		dropFile(
			getDropZone(dialog),
			new File(['{"GOOD":"1","bad/name":"2"}'], "errors.json", {
				type: "application/json",
			}),
		);

		await waitFor(() =>
			expect(dialog.getByText("Validation failed.")).toBeVisible(),
		);
		const entry = dialog.getByRole("listitem");
		expect(entry).toHaveTextContent(
			'secrets[1].nameSecret "bad/name": Name must not contain /, ?, or #.',
		);
		expect(entry).not.toHaveTextContent(/on line/i);
		expect(entry).not.toHaveTextContent("()");
	},
};

export const EditSecretHasNoDropZone: Story = {
	args: {
		secret: MockUserSecrets[0],
	},
	play: async ({ canvasElement }) => {
		const dialog = await findDialog(canvasElement);

		await waitFor(() =>
			expect(
				dialog.getByRole("heading", { name: "Edit secret" }),
			).toBeVisible(),
		);
		expect(
			dialog.queryByRole("button", { name: /Import secrets from a file/ }),
		).toBeNull();
	},
};
