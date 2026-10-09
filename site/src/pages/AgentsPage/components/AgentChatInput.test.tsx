import {
	act,
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
import { AgentComposer, useAgentComposer } from "./AgentComposer";

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
	initialValue: "",
	onContentChange: vi.fn(),
	planModeEnabled: false,
	onPlanModeToggle: vi.fn(),
	isModelCatalogLoading: false,
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
	it("shares submission with a sibling outside the visual frame", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn();
		const SubmitDraft = () => {
			const { actions } = useAgentComposer();
			return (
				<button type="button" onClick={actions.submit}>
					Submit draft
				</button>
			);
		};
		renderInput(
			<AgentComposer.Provider
				bindings={{
					onSend,
					isDisabled: false,
					isLoading: false,
					hasModelOptions: true,
					initialValue: "",
					onContentChange: vi.fn(),
				}}
			>
				<AgentComposer.Frame>
					<AgentComposer.Editor hasWorkspace={false} />
				</AgentComposer.Frame>
				<SubmitDraft />
			</AgentComposer.Provider>,
		);
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Shared draft");
		await user.click(screen.getByRole("button", { name: "Submit draft" }));
		expect(onSend).toHaveBeenCalledExactlyOnceWith("Shared draft");
	});

	it("fills its container when the page column sets the width", () => {
		localStorage.removeItem("agents.chat-full-width");
		const { rerender } = renderInput(<AgentChatInput {...inputProps} />);
		const column = screen.getByTestId("chat-composer").parentElement;

		expect(column).toHaveClass("max-w-3xl");

		rerender(
			<AppProviders>
				<AgentChatInput {...inputProps} fillWidth />
			</AppProviders>,
		);

		expect(screen.getByTestId("chat-composer").parentElement).toHaveClass(
			"max-w-full",
		);
	});

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
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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

	it.each([false, true])(
		"cycles prompt history and restores the draft without interrupting (streaming: %s)",
		async (isStreaming) => {
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			const onSend = vi.fn();
			const onInterrupt = vi.fn();
			const draft = "   ";
			renderInput(
				<AgentChatInput
					{...inputProps}
					inputRef={inputRef}
					onSend={onSend}
					onInterrupt={onInterrupt}
					isStreaming={isStreaming}
					initialValue={draft}
					userPromptHistory={["Latest prompt", "Older prompt"]}
				/>,
			);

			await user.click(screen.getByRole("textbox", { name: "Chat message" }));
			await waitFor(() => expect(inputRef.current?.getValue()).toBe(draft));
			await user.keyboard("{ArrowUp}");
			await waitFor(() =>
				expect(inputRef.current?.getValue()).toBe("Latest prompt"),
			);
			await user.keyboard("{ArrowUp}");
			await waitFor(() =>
				expect(inputRef.current?.getValue()).toBe("Older prompt"),
			);
			await user.keyboard("{ArrowUp}");
			expect(inputRef.current?.getValue()).toBe("Older prompt");
			await user.keyboard("{ArrowDown}");
			await waitFor(() =>
				expect(inputRef.current?.getValue()).toBe("Latest prompt"),
			);
			await user.keyboard("{ArrowDown}");
			await waitFor(() => expect(inputRef.current?.getValue()).toBe(draft));

			await user.keyboard("{ArrowUp}");
			await waitFor(() =>
				expect(inputRef.current?.getValue()).toBe("Latest prompt"),
			);
			await user.keyboard("{Escape}");
			await waitFor(() => expect(inputRef.current?.getValue()).toBe(draft));
			expect(onInterrupt).not.toHaveBeenCalled();
			expect(onSend).not.toHaveBeenCalled();

			await user.keyboard("{Escape}");
			expect(onInterrupt).toHaveBeenCalledTimes(isStreaming ? 1 : 0);
		},
	);

	it("does not replace a non-empty draft with prompt history", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				inputRef={inputRef}
				onSend={onSend}
				initialValue="Unfinished draft"
				userPromptHistory={["Latest prompt"]}
			/>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{ArrowUp}{ArrowDown}");
		expect(inputRef.current?.getValue()).toBe("Unfinished draft");
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(onSend).toHaveBeenCalledWith("Unfinished draft");
	});

	it("keeps the retained input ref live after restoring file references on remount", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const onContentChange =
			vi.fn<React.ComponentProps<typeof AgentChatInput>["onContentChange"]>();
		const props = { ...inputProps, inputRef, onSend, onContentChange };
		const { rerender } = renderInput(
			<AgentChatInput {...props} remountKey={0} />,
		);
		const handle = inputRef.current;
		const reference = {
			fileName: "src/main.ts",
			startLine: 2,
			endLine: 4,
			content: "export const answer = 42;",
		};

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Review this: ");
		act(() => handle?.addFileReference(reference));
		await waitFor(() => {
			expect(handle?.getContentParts()).toEqual([
				{ type: "text", text: "Review this: " },
				{ type: "file-reference", reference },
			]);
			expect(onContentChange).toHaveBeenLastCalledWith(
				expect.any(String),
				expect.any(String),
				true,
			);
		});
		const serializedState = onContentChange.mock.lastCall?.[1];
		if (!serializedState) throw new Error("Expected serialized editor state");
		act(() => handle?.clear());
		await waitFor(() => expect(handle?.getContentParts()).toEqual([]));

		rerender(
			<AppProviders>
				<AgentChatInput
					{...props}
					remountKey={1}
					initialValue="Plain-text fallback"
					initialEditorState={serializedState}
				/>
			</AppProviders>,
		);
		await waitFor(() => {
			expect(handle?.getContentParts()).toEqual([
				{ type: "text", text: "Review this: " },
				{ type: "file-reference", reference },
			]);
		});

		act(() => handle?.setValue("Replacement draft"));
		await waitFor(() => expect(handle?.getValue()).toBe("Replacement draft"));
		act(() => handle?.focus());
		await user.paste(" after remount");
		await waitFor(() =>
			expect(handle?.getValue()).toBe("Replacement draft after remount"),
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(onSend).toHaveBeenCalledWith("Replacement draft after remount");
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
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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
			onContentChange: vi.fn(),
			planModeEnabled: false,
			onPlanModeToggle: vi.fn(),
			isModelCatalogLoading: false,
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
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const onInterrupt = vi.fn();
		mockedUseSpeechRecognition.mockReturnValue({
			isSupported: true,
			isRecording: true,
			transcript: "Transcribed draft",
			error: null,
			start: vi.fn(),
			stop,
			cancel,
		});

		renderInput(
			<AgentChatInput
				inputRef={inputRef}
				onSend={onSend}
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
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
			/>,
		);

		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("Transcribed draft"),
		);
		await user.click(
			screen.getByRole("button", { name: "Accept voice input" }),
		);
		expect(stop).toHaveBeenCalledTimes(1);
		expect(onSend).not.toHaveBeenCalled();
		expect(onInterrupt).not.toHaveBeenCalled();
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
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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
				}}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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
				}}
				isDisabled
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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

	it("refuses workspace files while a send is pending", () => {
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
				}}
				isDisabled={false}
				isLoading
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
			/>,
		);

		// The post-send reset would discard the chip after the bytes
		// already landed, so the drop is refused with a wait message
		// rather than the "attach a workspace" one.
		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: {
				files: [createMockFile("dataset.zip", "application/zip")],
			},
		});

		expect(onWorkspaceAttach).not.toHaveBeenCalled();
		expect(onAttach).not.toHaveBeenCalled();
		expect(toastError).toHaveBeenCalledWith(
			"Wait for the current message to finish sending, then add the file again.",
		);
	});

	it("refuses pasted workspace files while a send is pending", () => {
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
				}}
				isDisabled={false}
				isLoading
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
			/>,
		);

		fireEvent.paste(screen.getByRole("textbox", { name: "Chat message" }), {
			clipboardData: {
				files: [createMockFile("dataset.zip", "application/zip")],
				types: ["Files"],
				getData: () => "",
			},
		});

		expect(onWorkspaceAttach).not.toHaveBeenCalled();
		expect(onAttach).not.toHaveBeenCalled();
		expect(toastError).toHaveBeenCalledWith(
			"Wait for the current message to finish sending, then add the file again.",
		);
	});

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
				}}
				isDisabled={false}
				isLoading={false}
				selectedModel={modelOptions[0].id}
				onModelChange={vi.fn()}
				modelOptions={modelOptions}
				modelSelectorPlaceholder="Select model"
				hasModelOptions
				canConfigureAgentSetup={false}
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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
