import {
	fireEvent,
	render,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { toast } from "sonner";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import type * as TypesGen from "#/api/typesGenerated";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { createMockFile } from "#/testHelpers/files";
import { mobileViewportMediaQuery } from "#/utils/mobile";
import type * as speechRecognition from "../hooks/useSpeechRecognition";
import { useSpeechRecognition } from "../hooks/useSpeechRecognition";
import { AgentChatInput, type ChatMessageInputRef } from "./AgentChatInput";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({ organizations: [] }),
}));

vi.mock("../hooks/useSpeechRecognition", async (importOriginal) => {
	const actual = await importOriginal<typeof speechRecognition>();
	return {
		...actual,
		useSpeechRecognition: vi.fn(actual.useSpeechRecognition),
	};
});

const mockedUseSpeechRecognition = vi.mocked(useSpeechRecognition);

// Captured once so repeated stubs within a test wrap the real
// implementation rather than a previous stub.
const originalMatchMedia = window.matchMedia;

const stubViewport = (isMobile: boolean) => {
	vi.stubGlobal("matchMedia", (query: string) => {
		const result = originalMatchMedia(query);
		return query === mobileViewportMediaQuery
			? { ...result, matches: isMobile }
			: result;
	});
};

const modelOptions = [
	{
		id: "model-config-1",
		provider: "openai",
		model: "gpt-4o",
		displayName: "GPT-4o",
	},
] as const;

const inputProps = {
	onSend: vi.fn(),
	isDisabled: false,
	isLoading: false,
	selectedModel: modelOptions[0].id,
	onModelChange: vi.fn(),
	modelOptions,
	modelSelectorPlaceholder: "Select model",
	hasModelOptions: true,
	canConfigureAgentSetup: false,
} satisfies React.ComponentProps<typeof AgentChatInput>;

const mockSentryMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-sentry",
	display_name: "Sentry",
	availability: "force_on",
	auth_type: "oauth2",
	auth_connected: true,
};

const mockLinearMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-linear",
	display_name: "Linear",
	availability: "default_on",
	auth_type: "api_key",
};

const mockGitHubMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-github",
	display_name: "GitHub",
	availability: "default_on",
	auth_type: "oauth2",
	auth_connected: true,
};

const mockGitHubMCPNeedingAuth: TypesGen.MCPServerConfig = {
	...mockGitHubMCP,
	auth_connected: false,
};

const mockNotionMCP: TypesGen.MCPServerConfig = {
	...MockMCPServerConfig,
	id: "mcp-notion",
	display_name: "Notion",
	availability: "default_on",
	auth_type: "api_key",
};

const mockMCPServers = [mockSentryMCP, mockLinearMCP, mockGitHubMCP];
const mockSelectedMCPServerIds = mockMCPServers.map((server) => server.id);

const renderInput = (children: React.ReactNode) => {
	return render(<AppProviders>{children}</AppProviders>);
};

beforeAll(() => {
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
});

afterEach(() => {
	vi.unstubAllGlobals();
	mockedUseSpeechRecognition.mockReset();
});

describe("AgentChatInput", () => {
	it("accepts drafts without sending while submission is disabled", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();

		renderInput(
			<AgentChatInput
				onSend={onSend}
				inputRef={inputRef}
				isDisabled
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Draft while models load");
		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("Draft while models load");
		});
		await user.keyboard("{Enter}");
		expect(onSend).not.toHaveBeenCalled();
	});

	it("attaches supported dropped files and reports unsupported ones", () => {
		const onAttach = vi.fn();
		const toastError = vi.spyOn(toast, "error");

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		const svg = createMockFile("diagram.svg", "image/svg+xml");
		const zip = createMockFile("archive.zip", "application/zip");
		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: { files: [svg, zip] },
		});

		expect(onAttach).toHaveBeenCalledWith([svg]);
		expect(toastError).toHaveBeenCalledWith(
			"Unsupported file type: archive.zip",
		);
	});

	it("removes a selected MCP server from the group", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={mockMCPServers}
				selectedMCPServerIds={mockSelectedMCPServerIds}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "3 MCPs" }));
		await user.click(
			within(screen.getByRole("dialog")).getByRole("button", {
				name: "Remove Linear",
			}),
		);
		expect(onMCPSelectionChange).toHaveBeenCalledWith([
			mockSentryMCP.id,
			mockGitHubMCP.id,
		]);
	});

	it("enables an unselected MCP server from the plus menu while the group is collapsed", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[...mockMCPServers, mockNotionMCP]}
				selectedMCPServerIds={mockSelectedMCPServerIds}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(
			await screen.findByRole("switch", { name: "Enable Notion" }),
		);
		expect(onMCPSelectionChange).toHaveBeenCalledWith([
			mockSentryMCP.id,
			mockLinearMCP.id,
			mockGitHubMCP.id,
			mockNotionMCP.id,
		]);
	});

	it("keeps two active MCP servers as individual pills", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[mockLinearMCP, mockGitHubMCP]}
				selectedMCPServerIds={[mockLinearMCP.id, mockGitHubMCP.id]}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Remove Linear" }));
		expect(onMCPSelectionChange).toHaveBeenCalledWith([mockGitHubMCP.id]);
	});

	it("excludes selected MCP servers that still need OAuth from the group", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[mockSentryMCP, mockLinearMCP, mockGitHubMCPNeedingAuth]}
				selectedMCPServerIds={[
					mockSentryMCP.id,
					mockLinearMCP.id,
					mockGitHubMCPNeedingAuth.id,
				]}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Remove Linear" }));
		expect(onMCPSelectionChange).toHaveBeenCalledWith([
			mockSentryMCP.id,
			mockGitHubMCPNeedingAuth.id,
		]);
	});

	it("allows viewing a disabled MCP group without changing its selection", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				isDisabled
				mcpServers={mockMCPServers}
				selectedMCPServerIds={mockSelectedMCPServerIds}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "3 MCPs" }));
		await user.click(
			within(screen.getByRole("dialog")).getByRole("button", {
				name: "Remove Linear",
			}),
		);
		expect(onMCPSelectionChange).not.toHaveBeenCalled();
	});

	it("swaps Stop for the Queue button once a draft is entered while streaming", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const onInterrupt = vi.fn();

		renderInput(
			<AgentChatInput
				onSend={onSend}
				inputRef={inputRef}
				isDisabled={false}
				isLoading={false}
				isStreaming
				onInterrupt={onInterrupt}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		expect(screen.queryByRole("button", { name: "Queue" })).toBeNull();
		await user.click(screen.getByRole("button", { name: "Stop" }));
		expect(onInterrupt).toHaveBeenCalledTimes(1);
		expect(onSend).not.toHaveBeenCalled();

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Also update the docs");
		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("Also update the docs");
		});

		expect(screen.queryByRole("button", { name: "Stop" })).toBeNull();
		await user.click(screen.getByRole("button", { name: "Queue" }));
		expect(onSend).toHaveBeenCalledWith("Also update the docs");
		expect(onInterrupt).toHaveBeenCalledTimes(1);

		inputRef.current?.clear();
		await waitFor(() => {
			expect(inputRef.current?.getValue()).toBe("");
		});

		expect(screen.queryByRole("button", { name: "Queue" })).toBeNull();
		await user.click(screen.getByRole("button", { name: "Stop" }));
		expect(onInterrupt).toHaveBeenCalledTimes(2);
	});

	it("shows a disabled Queue button without a spinner while an interrupt is pending", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn();

		renderInput(
			<AgentChatInput
				onSend={onSend}
				isDisabled={false}
				isLoading
				isStreaming
				isInterruptPending
				onInterrupt={vi.fn()}
				initialValue="Also update the docs"
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		const queueButton = screen.getByRole("button", { name: "Queue" });
		expect(queueButton).toBeDisabled();
		expect(within(queueButton).queryByTitle("Loading spinner")).toBeNull();
		await user.click(queueButton);
		expect(onSend).not.toHaveBeenCalled();
	});

	it("advertises the Enter send shortcut only on desktop viewports", () => {
		const props = {
			onSend: vi.fn(),
			isDisabled: false,
			isLoading: false,
			isStreaming: true,
			onInterrupt: vi.fn(),
			initialValue: "Also update the docs",
			selectedModel: modelOptions[0].id,
			onModelChange: vi.fn(),
			modelOptions,
			modelSelectorPlaceholder: "Select model",
			hasModelOptions: true,
			canConfigureAgentSetup: false,
		};

		stubViewport(false);
		const desktop = renderInput(<AgentChatInput {...props} />);
		expect(
			screen
				.getByRole("button", { name: "Queue" })
				.getAttribute("aria-keyshortcuts"),
		).toMatch(/Enter/);
		desktop.unmount();

		stubViewport(true);
		renderInput(<AgentChatInput {...props} />);
		expect(screen.getByRole("button", { name: "Queue" })).not.toHaveAttribute(
			"aria-keyshortcuts",
		);
	});

	it("keeps Accept and Cancel available while a recording outlives the start of a turn", async () => {
		const user = userEvent.setup();
		const stop = vi.fn();
		const cancel = vi.fn();
		mockedUseSpeechRecognition.mockReturnValue({
			isSupported: true,
			isRecording: true,
			transcript: "",
			error: null,
			start: vi.fn(),
			stop,
			cancel,
		});

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				isDisabled={false}
				isLoading={false}
				isStreaming
				onInterrupt={vi.fn()}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		expect(screen.queryByRole("button", { name: "Stop" })).toBeNull();
		await user.click(
			screen.getByRole("button", { name: "Accept voice input" }),
		);
		expect(stop).toHaveBeenCalledTimes(1);
		await user.click(
			screen.getByRole("button", { name: "Cancel voice input" }),
		);
		expect(cancel).toHaveBeenCalledTimes(1);
	});

	it("lets a recording start while a turn is streaming", async () => {
		const user = userEvent.setup();
		const start = vi.fn();
		mockedUseSpeechRecognition.mockReturnValue({
			isSupported: true,
			isRecording: false,
			transcript: "",
			error: null,
			start,
			stop: vi.fn(),
			cancel: vi.fn(),
		});

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				isDisabled={false}
				isLoading={false}
				isStreaming
				onInterrupt={vi.fn()}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Voice input" }));
		expect(start).toHaveBeenCalledTimes(1);
		expect(screen.getByRole("button", { name: "Stop" })).toBeEnabled();
	});

	it("falls back to pasted text when every pasted file is refused", async () => {
		const onAttach = vi.fn();
		const inputRef = createRef<ChatMessageInputRef>();

		renderInput(
			<AgentChatInput
				inputRef={inputRef}
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		const target = screen.getByRole("textbox", { name: "Chat message" });
		target.focus();
		// Clipboard carrying both a file and a text payload. Without
		// workspaceUploads the zip cannot be routed anywhere, so the
		// paste must fall back to inserting the clipboard text.
		fireEvent.paste(target, {
			clipboardData: {
				files: [createMockFile("dataset.zip", "application/zip")],
				types: ["Files", "text/plain"],
				getData: (type: string) =>
					type === "text/plain" ? "notes about the archive" : "",
			},
		});

		await waitFor(() => {
			expect(inputRef.current?.getValue()).toContain("notes about the archive");
		});
		expect(onAttach).not.toHaveBeenCalled();
	});

	it("routes workspace files to workspace uploads instead of attachments", () => {
		const onAttach = vi.fn();
		const onWorkspaceAttach = vi.fn();

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				workspaceUploads={{
					uploads: [],
					onAttach: onWorkspaceAttach,
					onRemove: vi.fn(),
					removeDisabled: false,
				}}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		const zip = createMockFile("dataset.zip", "application/zip");
		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: { files: [zip] },
		});

		expect(onWorkspaceAttach).toHaveBeenCalledWith([zip]);
		expect(onAttach).not.toHaveBeenCalled();
	});

	it("refuses workspace files while the composer is disabled", () => {
		const onAttach = vi.fn();
		const onWorkspaceAttach = vi.fn();
		const toastError = vi.spyOn(toast, "error");

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				workspaceUploads={{
					uploads: [],
					onAttach: onWorkspaceAttach,
					onRemove: vi.fn(),
					removeDisabled: false,
				}}
				isDisabled
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: {
				files: [createMockFile("dataset.zip", "application/zip")],
			},
		});

		expect(onWorkspaceAttach).not.toHaveBeenCalled();
		expect(onAttach).not.toHaveBeenCalled();
		expect(toastError).toHaveBeenCalledWith(
			"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.",
		);
	});

	it("refuses a dropped batch with one toast while a send is pending", () => {
		const onAttach = vi.fn();
		const onWorkspaceAttach = vi.fn();
		const toastError = vi.spyOn(toast, "error");

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				workspaceUploads={{
					uploads: [],
					onAttach: onWorkspaceAttach,
					onRemove: vi.fn(),
					removeDisabled: false,
				}}
				isDisabled={false}
				isLoading
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: {
				files: [
					createMockFile("screenshot.png", "image/png"),
					createMockFile("dataset.zip", "application/zip"),
				],
			},
		});

		expect(onWorkspaceAttach).not.toHaveBeenCalled();
		expect(onAttach).not.toHaveBeenCalled();
		expect(toastError).toHaveBeenCalledTimes(1);
		expect(toastError).toHaveBeenCalledWith(
			"Wait for the current message to finish sending, then add the file again.",
		);
	});

	it("refuses a multi-file paste with one toast while a send is pending", () => {
		const onAttach = vi.fn();
		const onWorkspaceAttach = vi.fn();
		const toastError = vi.spyOn(toast, "error");

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				workspaceUploads={{
					uploads: [],
					onAttach: onWorkspaceAttach,
					onRemove: vi.fn(),
					removeDisabled: false,
				}}
				isDisabled={false}
				isLoading
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		fireEvent.paste(screen.getByRole("textbox", { name: "Chat message" }), {
			clipboardData: {
				files: [
					createMockFile("first.png", "image/png"),
					createMockFile("second.png", "image/png"),
					createMockFile("dataset.zip", "application/zip"),
				],
				types: ["Files"],
				getData: () => "",
			},
		});

		expect(onWorkspaceAttach).not.toHaveBeenCalled();
		expect(onAttach).not.toHaveBeenCalled();
		expect(toastError).toHaveBeenCalledTimes(1);
		expect(toastError).toHaveBeenCalledWith(
			"Wait for the current message to finish sending, then add the file again.",
		);
	});

	it("routes a multi-file paste as one batch", () => {
		const onAttach = vi.fn();

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		const first = createMockFile("first.png", "image/png");
		const second = createMockFile("second.png", "image/png");
		fireEvent.paste(screen.getByRole("textbox", { name: "Chat message" }), {
			clipboardData: {
				files: [first, second],
				types: ["Files"],
				getData: () => "",
			},
		});

		expect(onAttach).toHaveBeenCalledTimes(1);
		expect(onAttach).toHaveBeenCalledWith([first, second]);
	});

	it("ignores attachment remove and paste inline while a send is pending", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const notes = createMockFile("notes.txt", "text/plain");
		const onRemoveAttachment = vi.fn();
		const onTextPreview = vi.fn();

		renderInput(
			<AgentChatInput
				inputRef={inputRef}
				onSend={vi.fn()}
				onAttach={vi.fn()}
				attachments={[notes]}
				onRemoveAttachment={onRemoveAttachment}
				onTextPreview={onTextPreview}
				textContents={new Map([[notes, "meeting notes"]])}
				isDisabled={false}
				isLoading
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Remove notes.txt" }));
		await user.click(screen.getByRole("button", { name: "Paste inline" }));
		expect(onRemoveAttachment).not.toHaveBeenCalled();
		expect(inputRef.current?.getValue()).toBe("");

		await user.click(screen.getByRole("button", { name: "View notes.txt" }));
		expect(onTextPreview).toHaveBeenCalledWith(
			"meeting notes",
			"notes.txt",
			"text/plain",
		);
	});

	it.each([
		{ phase: "starts", afterSend: { isLoading: true, attached: true } },
		{ phase: "finishes", afterSend: { isLoading: false, attached: false } },
	])(
		"drops an inline-text action whose content loads after a send $phase",
		async ({ afterSend }) => {
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			const notes = createMockFile("notes.txt", "text/plain");
			const onRemoveAttachment = vi.fn();
			let resolveFetch: (response: Response) => void = () => {};
			vi.stubGlobal(
				"fetch",
				vi.fn(
					() =>
						new Promise<Response>((resolve) => {
							resolveFetch = resolve;
						}),
				),
			);
			const renderComposer = (isLoading: boolean, attachments: File[]) => (
				<AppProviders>
					<AgentChatInput
						inputRef={inputRef}
						onSend={vi.fn()}
						onAttach={vi.fn()}
						attachments={attachments}
						onRemoveAttachment={onRemoveAttachment}
						uploadStates={
							new Map([[notes, { status: "uploaded", fileId: "file-notes" }]])
						}
						isDisabled={false}
						isLoading={isLoading}
						selectedModel={modelOptions[0].id}
						onModelChange={vi.fn()}
						modelOptions={modelOptions}
						modelSelectorPlaceholder="Select model"
						hasModelOptions
						canConfigureAgentSetup={false}
					/>
				</AppProviders>
			);
			const { rerender } = render(renderComposer(false, [notes]));

			await user.click(screen.getByRole("button", { name: "Paste inline" }));
			await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
			rerender(renderComposer(true, [notes]));
			rerender(
				renderComposer(afterSend.isLoading, afterSend.attached ? [notes] : []),
			);
			resolveFetch(new Response("meeting notes"));

			// Let the resolved load reach the inline handler.
			await new Promise((resolve) => setTimeout(resolve, 0));
			expect(onRemoveAttachment).not.toHaveBeenCalled();
			expect(inputRef.current?.getValue()).toBe("");
		},
	);

	it("asks for a workspace when workspace uploads are wired but unavailable", () => {
		const onAttach = vi.fn();
		const toastError = vi.spyOn(toast, "error");

		renderInput(
			<AgentChatInput
				onSend={vi.fn()}
				onAttach={onAttach}
				attachments={[]}
				workspaceUploads={{
					uploads: [],
					onAttach: undefined,
					onRemove: vi.fn(),
					removeDisabled: false,
				}}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
			/>,
		);

		const zip = createMockFile("archive.zip", "application/zip");
		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: { files: [zip] },
		});

		expect(onAttach).not.toHaveBeenCalled();
		expect(toastError).toHaveBeenCalledWith(
			"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.",
		);
	});
});
