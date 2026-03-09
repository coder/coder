import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { AgentChatInput, type UploadState } from "./AgentChatInput";

const defaultModelOptions = [
	{
		id: "openai:gpt-4o",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const meta: Meta<typeof AgentChatInput> = {
	title: "pages/AgentsPage/AgentChatInput",
	component: AgentChatInput,
	args: {
		onSend: fn(),
		onContentChange: fn(),
		onModelChange: fn(),
		initialValue: "",
		isDisabled: false,
		isLoading: false,
		selectedModel: defaultModelOptions[0].id,
		modelOptions: [...defaultModelOptions],
		modelSelectorPlaceholder: "Select model",
		hasModelOptions: true,
		inputStatusText: null,
		modelCatalogStatusMessage: null,
	},
};

export default meta;
type Story = StoryObj<typeof AgentChatInput>;

export const Default: Story = {};

export const DisablesSendUntilInput: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sendButton = canvas.getByRole("button", { name: "Send" });

		expect(sendButton).toBeDisabled();
	},
};

export const SendsAndClearsInput: Story = {
	args: {
		onSend: fn(),
		initialValue: "Run focused tests",
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);

		// Wait for the Lexical editor to initialize and render the
		// initial value text into the DOM before interacting.
		const editor = canvas.getByTestId("chat-message-input");
		await waitFor(() => {
			expect(editor.textContent).toBe("Run focused tests");
		});

		const sendButton = canvas.getByRole("button", { name: "Send" });
		await waitFor(() => {
			expect(sendButton).toBeEnabled();
		});

		await userEvent.click(sendButton);

		await waitFor(() => {
			expect(args.onSend).toHaveBeenCalledWith("Run focused tests");
		});
	},
};

export const DisabledInput: Story = {
	args: {
		isDisabled: true,
		initialValue: "Should not send",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();

		// The editor should be non-editable so users cannot click
		// into it and type (e.g. archived chats).
		const editor = canvas.getByTestId("chat-message-input");
		await waitFor(() => {
			expect(editor).toHaveAttribute("contenteditable", "false");
		});
	},
};

export const NoModelOptions: Story = {
	args: {
		isDisabled: false,
		hasModelOptions: false,
		initialValue: "Model required",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByRole("button", { name: "Send" })).toBeDisabled();
	},
};

export const LoadingSpinner: Story = {
	args: {
		isDisabled: true,
		isLoading: true,
		initialValue: "Sending...",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sendButton = canvas.getByRole("button", { name: "Send" });
		expect(sendButton).toBeDisabled();
		// The Loader2Icon renders with the animate-spin class when
		// isLoading is true.
		expect(sendButton.querySelector(".animate-spin")).toBeTruthy();
	},
};

export const LoadingDisablesSend: Story = {
	args: {
		isDisabled: false,
		isLoading: true,
		initialValue: "Another message",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const sendButton = canvas.getByRole("button", { name: "Send" });
		// The send button should be disabled while a previous send is
		// in-flight, even though the textarea has content.
		expect(sendButton).toBeDisabled();
	},
};

const longContent = Array.from(
	{ length: 60 },
	(_, i) =>
		`Line ${i + 1}: This is a long line of text used to test overflow and scrollability of the chat input editor.`,
).join("\n");

export const LongContentScrollable: Story = {
	args: {
		initialValue: longContent,
	},
};

// Tiny 1x1 transparent PNG as data URI for attachment previews.
const TINY_PNG =
	"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";

const createMockFile = (name: string, type: string) =>
	new File(["mock-data"], name, { type });

export const WithAttachments: Story = {
	args: (() => {
		const file1 = createMockFile("screenshot.png", "image/png");
		const file2 = createMockFile("diagram.jpg", "image/jpeg");
		const attachments = [file1, file2];
		return {
			attachments,
			uploadStates: new Map<File, UploadState>([
				[file1, { status: "uploaded", fileId: "f1" }],
				[file2, { status: "uploaded", fileId: "f2" }],
			]),
			previewUrls: new Map<File, string>([
				[file1, TINY_PNG],
				[file2, TINY_PNG],
			]),
			onAttach: fn(),
			onRemoveAttachment: fn(),
			initialValue: "Here are the images",
		};
	})(),
};

export const WithUploadingAttachment: Story = {
	args: (() => {
		const file = createMockFile("uploading.png", "image/png");
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[file, { status: "uploading" }],
			]),
			previewUrls: new Map<File, string>([[file, TINY_PNG]]),
			onAttach: fn(),
			onRemoveAttachment: fn(),
			initialValue: "Waiting for upload",
		};
	})(),
};

export const WithAttachmentError: Story = {
	args: (() => {
		const file = createMockFile("broken.png", "image/png");
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[file, { status: "error", error: "Upload failed: server error" }],
			]),
			previewUrls: new Map<File, string>([[file, TINY_PNG]]),
			onAttach: fn(),
			onRemoveAttachment: fn(),
			initialValue: "Upload had an error",
		};
	})(),
};

export const AttachmentsOnly: Story = {
	args: (() => {
		const file = createMockFile("photo.png", "image/png");
		return {
			attachments: [file],
			uploadStates: new Map<File, UploadState>([
				[file, { status: "uploaded", fileId: "f-only" }],
			]),
			previewUrls: new Map<File, string>([[file, TINY_PNG]]),
			onAttach: fn(),
			onRemoveAttachment: fn(),
			initialValue: "",
		};
	})(),
};
