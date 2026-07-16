import type { Meta, StoryObj } from "@storybook/react-vite";
import { MonitorDotIcon } from "lucide-react";
import { useEffect, useRef } from "react";
import { expect, fn, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatContextClean,
	MockMCPServerConfig,
} from "#/testHelpers/chatEntities";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { createMockFile } from "#/testHelpers/files";
import { withProxyProvider } from "#/testHelpers/storybook";
import {
	AgentChatInput,
	type AgentContextUsage,
	type UploadState,
} from "./AgentChatInput";
import type { ChatMessageInputRef } from "./ChatMessageInput/ChatMessageInput";

const defaultModelConfigID = "model-config-1";

const defaultModelOptions = [
	{
		id: defaultModelConfigID,
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const meta: Meta<typeof AgentChatInput> = {
	title: "pages/AgentsPage/AgentChatInput",
	component: AgentChatInput,
	decorators: [withProxyProvider()],
	args: {
		onSend: fn(),
		sendShortcut: "enter",
		onContentChange: fn(),
		onModelChange: fn(),
		initialValue: "",
		isDisabled: false,
		isLoading: false,
		selectedModel: defaultModelOptions[0].id,
		modelOptions: [...defaultModelOptions],
		modelSelectorPlaceholder: "Select model",
		hasModelOptions: true,
	},
};

export default meta;
type Story = StoryObj<typeof AgentChatInput>;

const promptHistory = [
	"Most recent prompt",
	"Middle prompt",
	"Oldest prompt",
] as const;

const getEditor = (canvasElement: HTMLElement) =>
	within(canvasElement).getByTestId("chat-message-input");

const expectEditorText = async (editor: HTMLElement, text: string) => {
	await waitFor(() => {
		expect(editor.textContent).toBe(text);
	});
};

export const Default: Story = {};

export const PromptHistoryCycling: Story = {
	args: {
		userPromptHistory: promptHistory,
	},
	play: async ({ canvasElement }) => {
		const editor = getEditor(canvasElement);
		await expectEditorText(editor, "");
		await userEvent.click(editor);

		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "Most recent prompt");
		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "Middle prompt");
		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "Oldest prompt");
		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "Oldest prompt");

		await userEvent.keyboard("{ArrowDown}");
		await expectEditorText(editor, "Middle prompt");
		await userEvent.keyboard("{ArrowDown}");
		await expectEditorText(editor, "Most recent prompt");
		await userEvent.keyboard("{ArrowDown}");
		await expectEditorText(editor, "");

		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "Most recent prompt");
		await userEvent.keyboard("{Escape}");
		await expectEditorText(editor, "");
	},
};

export const PromptHistoryCyclingExitsOnTyping: Story = {
	args: {
		userPromptHistory: promptHistory,
	},
	play: async ({ canvasElement }) => {
		const editor = getEditor(canvasElement);
		await expectEditorText(editor, "");
		await userEvent.click(editor);

		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "Most recent prompt");
		await userEvent.keyboard("!");
		await expectEditorText(editor, "Most recent prompt!");
		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "Most recent prompt!");

		await userEvent.keyboard("{Control>}a{/Control}{Backspace}");
		await expectEditorText(editor, "");
		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "Most recent prompt");
		await userEvent.keyboard("{ArrowDown}");
		await expectEditorText(editor, "");
	},
};

export const NoPromptHistoryUpArrowIsNoOp: Story = {
	args: {
		userPromptHistory: [],
	},
	play: async ({ canvasElement }) => {
		const editor = getEditor(canvasElement);
		await expectEditorText(editor, "");
		await userEvent.click(editor);
		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "");
	},
};

export const PromptHistorySuppressedWhileEditingHistoryMessage: Story = {
	args: {
		isEditingHistoryMessage: true,
		userPromptHistory: promptHistory,
	},
	play: async ({ canvasElement }) => {
		const editor = getEditor(canvasElement);
		await expectEditorText(editor, "");
		await userEvent.click(editor);
		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "");
	},
};

export const PromptHistorySuppressedWhileDisabled: Story = {
	args: {
		isDisabled: true,
		userPromptHistory: promptHistory,
	},
	play: async ({ canvasElement }) => {
		const editor = getEditor(canvasElement);
		await expectEditorText(editor, "");
		await userEvent.click(editor);
		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "");
	},
};

export const PromptHistorySuppressedWhileLoading: Story = {
	args: {
		isLoading: true,
		userPromptHistory: promptHistory,
	},
	play: async ({ canvasElement }) => {
		const editor = getEditor(canvasElement);
		await expectEditorText(editor, "");
		await userEvent.click(editor);
		await userEvent.keyboard("{ArrowUp}");
		await expectEditorText(editor, "");
	},
};

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

export const EnterSendsByDefault: Story = {
	args: {
		onSend: fn(),
		initialValue: "Run focused tests",
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const editor = canvas.getByTestId("chat-message-input");
		await waitFor(() => {
			expect(editor.textContent).toBe("Run focused tests");
		});

		await userEvent.click(editor);
		await userEvent.keyboard("{Enter}");

		await waitFor(() => {
			expect(args.onSend).toHaveBeenCalledWith("Run focused tests");
		});
	},
};

export const ModifierEnterSendsWhenRequired: Story = {
	args: {
		onSend: fn(),
		sendShortcut: "modifier_enter",
		initialValue: "Run focused tests",
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const editor = canvas.getByTestId("chat-message-input");
		await waitFor(() => {
			expect(editor.textContent).toBe("Run focused tests");
		});

		await userEvent.click(editor);
		await userEvent.keyboard("{Enter}");
		expect(args.onSend).not.toHaveBeenCalled();
		await waitFor(() => {
			expect(editor.querySelectorAll("br").length).toBeGreaterThan(0);
		});

		await userEvent.keyboard("{Control>}{Enter}{/Control}");
		await waitFor(() => {
			expect(args.onSend).toHaveBeenCalledWith("Run focused tests");
		});
	},
};

/**
 * CODAGT-210: On mobile viewports, Enter must insert a newline rather
 * than submit the message, because Shift+Enter is cumbersome on
 * on-screen keyboards. Users submit via the send button instead.
 */
export const MobileEnterInsertsNewline: Story = {
	args: {
		onSend: fn(),
		initialValue: "Line one",
	},
	play: async ({ canvasElement, args }) => {
		const originalMatchMedia = window.matchMedia;
		window.matchMedia = (query: string) =>
			({
				matches: query === "(max-width: 639px)",
				media: query,
				onchange: null,
				addEventListener: () => undefined,
				removeEventListener: () => undefined,
				dispatchEvent: () => true,
				addListener: () => undefined,
				removeListener: () => undefined,
			}) as MediaQueryList;

		try {
			const canvas = within(canvasElement);
			const editor = canvas.getByTestId("chat-message-input");
			await waitFor(() => {
				expect(editor.textContent).toBe("Line one");
			});

			await userEvent.click(editor);
			await userEvent.keyboard("{Enter}");

			expect(args.onSend).not.toHaveBeenCalled();
		} finally {
			window.matchMedia = originalMatchMedia;
		}
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

export const AIGatewayDisabledShowsSetupNotice: Story = {
	args: {
		// canConfigureAgentSetup: false and providerCount/modelCount left
		// undefined simulates the model-catalog query still loading, which
		// used to make an admin briefly see the wrong copy before this was
		// fixed to short-circuit on aiGatewayDisabled directly.
		canConfigureAgentSetup: false,
		aiGatewayDisabled: true,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText(/Enable it in your deployment config/),
		).toBeInTheDocument();
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
		// The Spinner component renders an SVG with a "Loading spinner"
		// title when isLoading is true.
		const spinnerSvg = sendButton.querySelector("svg");
		expect(spinnerSvg).toBeTruthy();
		expect(spinnerSvg?.querySelector("title")?.textContent).toBe(
			"Loading spinner",
		);
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

export const Streaming: Story = {
	args: {
		isStreaming: true,
		onInterrupt: fn(),
		isInterruptPending: false,
		initialValue: "",
		onAttach: fn(),
		onRemoveAttachment: fn(),
	},
};

export const StreamingInterruptPending: Story = {
	args: {
		isStreaming: true,
		onInterrupt: fn(),
		isInterruptPending: true,
		initialValue: "",
		onAttach: fn(),
		onRemoveAttachment: fn(),
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

export const UploadingDisablesSend: Story = {
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
			initialValue: "Message with uploading image",
		};
	})(),
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		// Send should be disabled while an upload is still in progress,
		// even though the editor has text content.
		const sendButton = canvas.getByRole("button", { name: "Send" });
		expect(sendButton).toBeDisabled();
		// Enter key should not trigger send while uploading.
		const editor = canvas.getByRole("textbox");
		await userEvent.click(editor);
		await userEvent.keyboard("{Enter}");
		expect(args.onSend).not.toHaveBeenCalled();
	},
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

/** File reference chip rendered inline with text in the editor. */
export const WithFileReference: Story = {
	render: (args) => {
		const ref = useRef<ChatMessageInputRef>(null);

		useEffect(() => {
			const handle = ref.current;
			if (!handle) return;
			handle.addFileReference({
				fileName: "site/src/components/Button.tsx",
				startLine: 42,
				endLine: 42,
				content: "export const Button = ...",
			});
		}, []);

		return <AgentChatInput {...args} inputRef={ref} />;
	},
	args: {
		initialValue: "Can you refactor ",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText(/Button\.tsx/)).toBeInTheDocument();
		});
	},
};

/** Multiple file reference chips rendered inline with text. */
export const WithMultipleFileReferences: Story = {
	render: (args) => {
		const ref = useRef<ChatMessageInputRef>(null);

		useEffect(() => {
			const handle = ref.current;
			if (!handle) return;
			handle.addFileReference({
				fileName: "api/handler.go",
				startLine: 1,
				endLine: 50,
				content: "...",
			});
			handle.insertText(" and ");
			handle.addFileReference({
				fileName: "api/handler_test.go",
				startLine: 10,
				endLine: 30,
				content: "...",
			});
		}, []);

		return <AgentChatInput {...args} inputRef={ref} />;
	},
	args: {
		initialValue: "Compare ",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(canvas.getByText(/handler\.go/)).toBeInTheDocument();
			expect(canvas.getByText(/handler_test\.go/)).toBeInTheDocument();
		});
	},
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

const LARGE_PASTE_MARKER = "__PASTE_MARKER_TEST__";

const largePasteText = Array.from({ length: 12 }, (_, i) =>
	i === 6 ? LARGE_PASTE_MARKER : `line ${i + 1} of pasted content`,
).join("\n");

function dispatchPasteWithText(element: HTMLElement, text: string): void {
	const dt = new DataTransfer();
	dt.setData("text/plain", text);
	const event = new ClipboardEvent("paste", {
		bubbles: true,
		cancelable: true,
	});
	Object.defineProperty(event, "clipboardData", {
		value: dt,
		writable: false,
	});
	element.dispatchEvent(event);
}

function getPasteTarget(container: HTMLElement): HTMLElement {
	const element = container.querySelector(
		'[data-testid="chat-message-input"]',
	) as HTMLElement;
	if (element?.getAttribute("contenteditable") === "true") {
		return element;
	}

	const contentEditable = element?.querySelector(
		'[contenteditable="true"]',
	) as HTMLElement;
	return contentEditable ?? element;
}

export const LargePasteCreatesAttachmentPreview: Story = {
	args: {
		attachments: [],
		onAttach: fn(),
		onRemoveAttachment: fn(),
	},
	parameters: {
		pixel: { exclude: true },
	},
	play: async ({ canvasElement, args }) => {
		const target = getPasteTarget(canvasElement);
		await waitFor(() => {
			expect(target.getAttribute("contenteditable")).toBe("true");
		});
		target.focus();

		dispatchPasteWithText(target, largePasteText);

		await waitFor(() => {
			expect(args.onAttach).toHaveBeenCalledTimes(1);
		});

		const callArgs = (args.onAttach as ReturnType<typeof fn>).mock.calls[0];
		const files = callArgs[0] as File[];
		expect(files).toHaveLength(1);
		expect(files[0].type).toBe("text/plain");
		expect(files[0].name).toMatch(
			/^pasted-text-\d{4}-\d{2}-\d{2}-\d{2}-\d{2}-\d{2}\.txt$/,
		);
		expect(target.textContent).not.toContain(LARGE_PASTE_MARKER);
	},
};

export const CtrlShiftVBypassesAttachmentCollapse: Story = {
	args: {
		attachments: [],
		onAttach: fn(),
		onRemoveAttachment: fn(),
	},
	parameters: {
		pixel: { exclude: true },
	},
	play: async ({ canvasElement, args }) => {
		const target = getPasteTarget(canvasElement);
		await waitFor(() => {
			expect(target.getAttribute("contenteditable")).toBe("true");
		});
		target.focus();

		const keyDown = new KeyboardEvent("keydown", {
			key: "v",
			code: "KeyV",
			shiftKey: true,
			ctrlKey: true,
			metaKey: false,
			bubbles: true,
			cancelable: true,
		});
		target.dispatchEvent(keyDown);
		dispatchPasteWithText(target, largePasteText);

		await waitFor(() => {
			expect(target.textContent).toContain(LARGE_PASTE_MARKER);
		});

		expect(args.onAttach).not.toHaveBeenCalled();
	},
};

// ── MCP server fixtures ────────────────────────────────────────

const now = "2026-03-19T12:00:00.000Z";

const buildMCPServer = (
	overrides: Partial<TypesGen.MCPServerConfig> &
		Pick<TypesGen.MCPServerConfig, "id" | "display_name" | "slug">,
): TypesGen.MCPServerConfig => ({
	...MockMCPServerConfig,
	created_at: now,
	updated_at: now,
	...overrides,
});

const sentryMCP = buildMCPServer({
	id: "mcp-sentry",
	display_name: "Sentry",
	slug: "sentry",
	icon_url: "/icon/widgets.svg",
	availability: "force_on",
	auth_type: "oauth2",
	auth_connected: true,
	enabled: true,
});

const linearMCP = buildMCPServer({
	id: "mcp-linear",
	display_name: "Linear",
	slug: "linear",
	availability: "default_on",
	auth_type: "api_key",
	enabled: true,
});

const githubMCP = buildMCPServer({
	id: "mcp-github",
	display_name: "GitHub",
	slug: "github",
	icon_url: "/icon/github.svg",
	availability: "default_on",
	auth_type: "oauth2",
	auth_connected: false,
	enabled: true,
});

const githubMCPConnected = { ...githubMCP, auth_connected: true };

const notionMCPConnected = buildMCPServer({
	id: "mcp-notion",
	display_name: "Notion",
	slug: "notion",
	availability: "default_on",
	auth_type: "oauth2",
	auth_connected: true,
	enabled: true,
});

const mcpDefaults = {
	onMCPSelectionChange: fn(),
	onMCPAuthComplete: fn(),
};

// ── MCP stories ────────────────────────────────────────────────

/** Input with multiple MCP servers selected — shows icon stack in toolbar. */
export const WithMCPServers: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [sentryMCP, linearMCP, githubMCPConnected],
		selectedMCPServerIds: [sentryMCP.id, linearMCP.id, githubMCPConnected.id],
	},
};

/** MCP server needing OAuth — shows Auth button instead of toggle. */
export const WithMCPNeedingAuth: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [sentryMCP, githubMCP],
		selectedMCPServerIds: [sentryMCP.id, githubMCP.id],
	},
};

/** No MCP servers active — shows only "MCP" label with chevron. */
export const WithMCPNoneActive: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [
			{
				...sentryMCP,
				availability: "default_off",
				auth_connected: false,
			},
			{
				...linearMCP,
				availability: "default_off",
				auth_type: "oauth2",
				auth_connected: false,
			},
		],
		selectedMCPServerIds: [],
	},
};

/** Plus menu open showing attach, MCP servers, and workspace placeholder. */
export const PlusMenuOpen: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [sentryMCP, linearMCP, githubMCPConnected],
		selectedMCPServerIds: [sentryMCP.id, linearMCP.id, githubMCPConnected.id],
		onAttach: fn(),
		onRemoveAttachment: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
	},
};

export const MCPDisconnectControls: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [linearMCP, githubMCP, notionMCPConnected],
		selectedMCPServerIds: [linearMCP.id, notionMCPConnected.id],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		expect(
			await body.findByRole("button", { name: "Disconnect Notion" }),
		).toBeInTheDocument();
		expect(
			body.queryByRole("button", { name: "Disconnect GitHub" }),
		).not.toBeInTheDocument();
		expect(body.getByRole("button", { name: "Auth" })).toBeInTheDocument();
		expect(
			body.queryByRole("button", { name: "Disconnect Linear" }),
		).not.toBeInTheDocument();
	},
};

export const MCPDisconnectCancel: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [githubMCPConnected],
		selectedMCPServerIds: [githubMCPConnected.id],
	},
	beforeEach: () => {
		spyOn(API.experimental, "disconnectMCPServerOAuth2").mockResolvedValue();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await userEvent.click(
			await body.findByRole("button", { name: "Disconnect GitHub" }),
		);
		expect(await body.findByText("Disconnect GitHub?")).toBeInTheDocument();
		await userEvent.click(body.getByRole("button", { name: "Cancel" }));
		await waitFor(() =>
			expect(body.queryByText("Disconnect GitHub?")).not.toBeInTheDocument(),
		);
		expect(API.experimental.disconnectMCPServerOAuth2).not.toHaveBeenCalled();
	},
};

export const MCPDisconnectConfirm: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [githubMCPConnected],
		selectedMCPServerIds: [githubMCPConnected.id],
	},
	beforeEach: () => {
		spyOn(API.experimental, "disconnectMCPServerOAuth2").mockResolvedValue();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await userEvent.click(
			await body.findByRole("button", { name: "Disconnect GitHub" }),
		);
		await body.findByText("Disconnect GitHub?");
		await userEvent.click(body.getByRole("button", { name: "Disconnect" }));
		await waitFor(() =>
			expect(body.queryByText("Disconnect GitHub?")).not.toBeInTheDocument(),
		);
		expect(API.experimental.disconnectMCPServerOAuth2).toHaveBeenCalledTimes(1);
		expect(API.experimental.disconnectMCPServerOAuth2).toHaveBeenCalledWith(
			githubMCPConnected.id,
		);
	},
};

export const MCPDisconnectError: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [githubMCPConnected],
		selectedMCPServerIds: [githubMCPConnected.id],
	},
	beforeEach: () => {
		spyOn(API.experimental, "disconnectMCPServerOAuth2").mockRejectedValue(
			new Error("disconnect failed"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await userEvent.click(
			await body.findByRole("button", { name: "Disconnect GitHub" }),
		);
		await body.findByText("Disconnect GitHub?");
		await userEvent.click(body.getByRole("button", { name: "Disconnect" }));
		await waitFor(() =>
			expect(API.experimental.disconnectMCPServerOAuth2).toHaveBeenCalled(),
		);
		expect(body.getByText("Disconnect GitHub?")).toBeInTheDocument();
	},
};

export const PlanFirstMenuItem: Story = {
	args: {
		onPlanModeToggle: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await body.findByRole("dialog");
		const toggles = await body.findAllByRole("menuitemcheckbox", {
			name: "Plan first",
		});
		const toggle = toggles.at(-1)!;
		expect(toggle).toBeInTheDocument();
	},
};

export const PlanningIndicator: Story = {
	args: {
		planModeEnabled: true,
		onPlanModeToggle: fn(),
	},
	parameters: {
		viewport: { defaultViewport: "desktopZoom200" },
		chromatic: { viewports: [720] },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Planning")).toBeVisible();
		expect(
			canvas.getByRole("button", { name: "Disable plan mode" }),
		).toBeVisible();
	},
};

const narrowPlanningContextUsage: AgentContextUsage = {
	usedTokens: 100_000,
	contextLimitTokens: 200_000,
};

const narrowPlanningModelOptions = [
	{
		id: "long-model-name",
		provider: "anthropic",
		model: "claude-sonnet-4-5-long-name",
		displayName: "Claude Sonnet 4.5 Extended Thinking",
	},
] as const;

export const PlanningIndicatorNarrow: Story = {
	args: {
		planModeEnabled: true,
		onPlanModeToggle: fn(),
		contextUsage: narrowPlanningContextUsage,
		selectedModel: narrowPlanningModelOptions[0].id,
		modelOptions: [...narrowPlanningModelOptions],
	},
	decorators: [
		(Story) => (
			<div style={{ width: 360 }}>
				<Story />
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const composer = await canvas.findByTestId("chat-composer");
		const sendButton = canvas.getByRole("button", { name: "Send" });
		const contextUsageButton = canvas.getByRole("button", {
			name: /Context usage/,
		});
		const planningBadge = canvasElement.querySelector<HTMLElement>(
			"[data-testid='planning-badge']",
		);
		const isVisible = (element: HTMLElement) => {
			const style = getComputedStyle(element);
			const rect = element.getBoundingClientRect();
			return (
				style.display !== "none" &&
				style.visibility !== "hidden" &&
				rect.width > 0 &&
				rect.height > 0
			);
		};

		await waitFor(() => {
			const composerRect = composer.getBoundingClientRect();
			const sendButtonRect = sendButton.getBoundingClientRect();
			const contextUsageRect = contextUsageButton.getBoundingClientRect();

			expect(contextUsageRect.left).toBeGreaterThanOrEqual(composerRect.left);
			expect(sendButtonRect.right).toBeLessThanOrEqual(composerRect.right);

			if (planningBadge && isVisible(planningBadge)) {
				expect(planningBadge.getBoundingClientRect().right).toBeLessThanOrEqual(
					contextUsageRect.left + 1,
				);
				return;
			}

			expect(canvas.getByRole("button", { name: "1 more item" })).toBeVisible();
		});
	},
};

export const DisablePlanModeFromBadge: Story = {
	args: {
		planModeEnabled: true,
		onPlanModeToggle: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const dismiss = canvas.getByRole("button", {
			name: "Disable plan mode",
		});
		await userEvent.click(dismiss);
		expect(args.onPlanModeToggle).toHaveBeenCalledTimes(1);
		expect(args.onPlanModeToggle).toHaveBeenCalledWith(false);
	},
};

export const PlanningIndicatorWithoutToggle: Story = {
	args: {
		planModeEnabled: true,
		onPlanModeToggle: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		expect(canvas.getByText("Planning")).toBeVisible();
		expect(
			canvas.queryByRole("button", { name: "Disable plan mode" }),
		).not.toBeInTheDocument();
	},
};

export const PlanFirstCheckedState: Story = {
	args: {
		planModeEnabled: true,
		onPlanModeToggle: fn(),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(canvas.getByRole("button", { name: "More options" }));
		await body.findByRole("dialog");
		const toggles = await body.findAllByRole("menuitemcheckbox", {
			name: "Plan first",
		});
		const toggle = toggles.at(-1)!;
		expect(toggle).toHaveAttribute("aria-checked", "true");
	},
};

export const DetailPageWorkspacePicker: Story = {
	args: {
		workspaceOptions: [
			{
				id: "ws-detail",
				name: "agents-workspace",
				owner_name: "mike",
				organization_id: "org-1",
			},
		],
		selectedWorkspaceId: "ws-detail",
		onWorkspaceChange: fn(),
		attachedWorkspace: {
			id: "ws-detail",
			name: "agents-workspace",
			route: "/@mike/agents-workspace",
			statusIcon: <MonitorDotIcon className="size-3" />,
			statusLabel: "Workspace running",
		},
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.getAllByText("agents-workspace")).toHaveLength(1);
		const removeWorkspaceButton = canvas.getByRole("button", {
			name: "Remove workspace agents-workspace",
		});
		expect(removeWorkspaceButton).toBeVisible();
		await userEvent.click(removeWorkspaceButton);
		expect(args.onWorkspaceChange).toHaveBeenCalledWith(null);

		const moreOptionsButton = canvas.getByRole("button", {
			name: "More options",
		});
		await userEvent.click(moreOptionsButton);
		await waitFor(() => {
			const plusMenuId = moreOptionsButton.getAttribute("aria-controls");
			if (!plusMenuId) {
				throw new Error("Expected More options to control a menu dialog.");
			}

			const plusMenu = canvasElement.ownerDocument.getElementById(plusMenuId);
			if (!(plusMenu instanceof HTMLElement)) {
				throw new Error("Expected More options menu dialog to render.");
			}

			expect(within(plusMenu).getByText("Attach workspace")).toBeVisible();
		});
	},
};

export const LinkedWorkspaceRemoveWhenInputDisabled: Story = {
	args: {
		isDisabled: true,
		workspace: MockWorkspace,
		workspaceAgent: MockWorkspaceAgent,
		chatId: "chat-detail",
		selectedWorkspaceId: MockWorkspace.id,
		onWorkspaceChange: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const workspaceMenuButton = canvas.getByRole("button", {
			name: `${MockWorkspace.name} workspace menu`,
		});

		expect(
			canvas.queryByRole("button", {
				name: `Remove workspace ${MockWorkspace.name}`,
			}),
		).not.toBeInTheDocument();
		expect(workspaceMenuButton).toBeVisible();
		expect(workspaceMenuButton).toBeEnabled();
		await userEvent.click(workspaceMenuButton);
		let detachWorkspaceItem: HTMLElement | null = null;
		await waitFor(() => {
			const menuId = workspaceMenuButton.getAttribute("aria-controls");
			if (!menuId) {
				throw new Error("Expected workspace pill to control a menu.");
			}

			const menu = canvasElement.ownerDocument.getElementById(menuId);
			if (!(menu instanceof HTMLElement)) {
				throw new Error("Expected workspace menu to render.");
			}

			detachWorkspaceItem = within(menu).getByRole("menuitem", {
				name: "Detach workspace",
			});
			expect(detachWorkspaceItem).toBeVisible();
		});
		if (!detachWorkspaceItem) {
			throw new Error("Expected detach workspace menu item to render.");
		}

		await userEvent.click(detachWorkspaceItem);
		expect(args.onWorkspaceChange).toHaveBeenCalledWith(null);
	},
};

export const UncheckSelectedWorkspaceFromPicker: Story = {
	args: {
		isDisabled: true,
		workspace: MockWorkspace,
		workspaceAgent: MockWorkspaceAgent,
		chatId: "chat-detail",
		workspaceOptions: [
			{
				id: MockWorkspace.id,
				name: MockWorkspace.name,
				owner_name: MockWorkspace.owner_name,
				organization_id: MockWorkspace.organization_id,
			},
		],
		selectedWorkspaceId: MockWorkspace.id,
		onWorkspaceChange: fn(),
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		chromatic: { viewports: [375] },
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);

		const moreOptionsButton = canvas.getByRole("button", {
			name: "More options",
		});
		expect(moreOptionsButton).toBeEnabled();
		await userEvent.click(moreOptionsButton);

		const attachWorkspaceButton = (
			await body.findByText("Attach workspace")
		).closest("button");
		if (!(attachWorkspaceButton instanceof HTMLButtonElement)) {
			throw new Error("Expected Attach workspace to be a button.");
		}
		expect(attachWorkspaceButton).toBeEnabled();
		await userEvent.click(attachWorkspaceButton);

		const workspaceMatches = await body.findAllByText(MockWorkspace.name);
		const selectedWorkspaceOption = workspaceMatches.at(-1);
		if (!(selectedWorkspaceOption instanceof HTMLElement)) {
			throw new Error("Expected workspace option to render.");
		}
		await userEvent.click(selectedWorkspaceOption);

		expect(args.onWorkspaceChange).toHaveBeenCalledWith(null);
	},
};

const confluenceMCP = buildMCPServer({
	id: "mcp-confluence",
	display_name: "Confluence Cloud",
	slug: "confluence",
	availability: "default_on",
	auth_type: "none",
	enabled: true,
});

const datadogMCP = buildMCPServer({
	id: "mcp-datadog",
	display_name: "Datadog Monitoring",
	slug: "datadog",
	availability: "default_on",
	auth_type: "none",
	enabled: true,
});

const pagerdutyMCP = buildMCPServer({
	id: "mcp-pagerduty",
	display_name: "PagerDuty",
	slug: "pagerduty",
	availability: "default_on",
	auth_type: "none",
	enabled: true,
});

/** Many tools with a workspace at 414px — forces overflow and "+N" pill. */
export const OverflowBadges: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [
			sentryMCP,
			linearMCP,
			githubMCPConnected,
			confluenceMCP,
			datadogMCP,
			pagerdutyMCP,
		],
		selectedMCPServerIds: [
			sentryMCP.id,
			linearMCP.id,
			githubMCPConnected.id,
			confluenceMCP.id,
			datadogMCP.id,
			pagerdutyMCP.id,
		],
		workspaceOptions: [
			{
				id: "ws-1",
				name: "my-long-workspace-name",
				owner_name: "admin",
				organization_id: "org-1",
			},
		],
		selectedWorkspaceId: "ws-1",
		onWorkspaceChange: fn(),
	},
	parameters: {
		viewport: { defaultViewport: "mobile2" },
		chromatic: { viewports: [414] },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// Wait for the overflow hook to measure and show the pill.
		const pill = await canvas.findByRole("button", {
			name: /more item/,
		});
		await waitFor(() => {
			expect(pill).toBeVisible();
		});
		await userEvent.click(pill);
		// The popover renders via a Radix portal outside the
		// canvas. Find it by role, then assert content within it.
		const popover = await within(document.body).findByRole("dialog");
		expect(within(popover).getByText("Confluence Cloud")).toBeInTheDocument();
	},
};

// ---------------------------------------------------------------------------
// Context-usage indicator stories
// ---------------------------------------------------------------------------

const baseContextUsage: AgentContextUsage = {
	usedTokens: 45_000,
	contextLimitTokens: 128_000,
	inputTokens: 30_000,
	outputTokens: 10_000,
	cacheReadTokens: 3_000,
	cacheCreationTokens: 2_000,
	compressionThreshold: 90,
};

/** Shows the context-usage ring and token summary tooltip. */
export const WithContextUsage: Story = {
	args: {
		contextUsage: baseContextUsage,
	},
};

/** Tooltip lists the chat's pinned context resources. */
export const WithContextFiles: Story = {
	args: {
		contextUsage: {
			...baseContextUsage,
			context: MockChatContextClean,
		},
	},
};

/** Context at 95%+ shows the ring in destructive (red) tone. */
export const ContextNearLimit: Story = {
	args: {
		contextUsage: {
			usedTokens: 124_000,
			contextLimitTokens: 128_000,
			inputTokens: 100_000,
			outputTokens: 20_000,
			cacheReadTokens: 4_000,
			compressionThreshold: 90,
		},
	},
};

/** Long workspace name at iPhone SE width — verifies truncation. */
export const LongWorkspaceNameMobile: Story = {
	args: {
		...mcpDefaults,
		mcpServers: [githubMCPConnected],
		selectedMCPServerIds: [githubMCPConnected.id],
		workspace: {
			...MockWorkspace,
			name: "my-super-extremely-long-workspace-name-that-overflows",
		},
		workspaceAgent: MockWorkspaceAgent,
		chatId: "test-chat-id",
	},
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		chromatic: { viewports: [375] },
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		// The workspace pill button should be present.
		const pill = await canvas.findByRole("button", {
			name: /workspace menu/,
		});
		await waitFor(() => {
			expect(pill).toBeVisible();
		});
		// The toolbar row should not cause horizontal overflow.
		const toolbar = pill.closest(
			".flex.items-center.justify-between",
		) as HTMLElement;
		if (toolbar?.parentElement) {
			expect(toolbar.scrollWidth).toBeLessThanOrEqual(
				toolbar.parentElement.clientWidth,
			);
		}
	},
};
