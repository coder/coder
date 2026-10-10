import {
	act,
	fireEvent,
	render,
	screen,
	waitFor,
	within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef, StrictMode } from "react";
import { toast } from "sonner";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockChatQueuedMessage,
	MockMCPServerConfig,
} from "#/testHelpers/chatEntities";
import { createDeferred } from "#/testHelpers/deferred";
import { createMockFile } from "#/testHelpers/files";
import { mobileViewportMediaQuery } from "#/utils/mobile";
import type * as speechRecognition from "../hooks/useSpeechRecognition";
import { useSpeechRecognition } from "../hooks/useSpeechRecognition";
import type { WorkspaceFileUpload } from "../hooks/useWorkspaceFileUploads";
import {
	AgentComposer,
	AgentComposerRuntimeProvider,
	useAgentComposer,
} from "./AgentComposer";
import { ChatComposer, LoadingChatComposer } from "./AgentComposers";
import type { ChatMessageInputRef } from "./ChatMessageInput/ChatMessageInput";

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
	bindings: {
		onSend: vi.fn(),
		isDisabled: false,
		isLoading: false,
		initialValue: "",
		onContentChange: vi.fn(),
		files: {
			workspaceUploads: { uploads: [], onRemove: vi.fn() },
			attachments: [],
			onRemoveAttachment: vi.fn(),
			uploadStates: new Map(),
			previewUrls: new Map(),
			textContents: new Map(),
		},
	},
	model: {
		selectedModel: modelOptions[0].id,
		onModelChange: vi.fn(),
		modelOptions,
		modelSelectorPlaceholder: "Select model",
		isModelCatalogLoading: false,
	},
	tools: {
		planning: { enabled: false, onChange: vi.fn() },
	},
	setup: { canConfigureAgentSetup: false },
} satisfies React.ComponentProps<typeof ChatComposer>;

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

const renderInput = (children: React.ReactNode) =>
	render(children, { wrapper: AppProviders });

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

describe("ChatComposer", () => {
	it("submits the editor draft from a sibling outside the frame", async () => {
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
			<AgentComposerRuntimeProvider
				bindings={{ ...inputProps.bindings, onSend }}
			>
				<AgentComposer.Frame>
					<AgentComposer.Editor hasWorkspace={false} />
				</AgentComposer.Frame>
				<SubmitDraft />
			</AgentComposerRuntimeProvider>,
		);
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Shared draft");
		await user.click(screen.getByRole("button", { name: "Submit draft" }));
		expect(onSend).toHaveBeenCalledExactlyOnceWith("Shared draft");
	});

	it.each([
		{ isDisabled: false, isEditingHistoryMessage: false, promoted: true },
		{ isDisabled: true, isEditingHistoryMessage: false, promoted: false },
		{ isDisabled: false, isEditingHistoryMessage: true, promoted: false },
	])(
		"handles empty Enter with the canonical queue (%o)",
		async ({ isDisabled, isEditingHistoryMessage, promoted }) => {
			const user = userEvent.setup();
			const onSend = vi.fn();
			const onPromote = vi.fn();
			renderInput(
				<ChatComposer
					{...inputProps}
					bindings={{
						...inputProps.bindings,
						onSend,
						isDisabled,
						isEditingHistoryMessage,
					}}
					queue={{
						messages: [
							MockChatQueuedMessage,
							{ ...MockChatQueuedMessage, id: 2 },
						],
						automationNames: { names: new Map(), status: "settled" },
						onDelete: vi.fn(),
						onPromote,
					}}
				/>,
			);
			await user.click(screen.getByRole("textbox", { name: "Chat message" }));
			await user.keyboard("{Enter}");
			if (promoted) {
				expect(onPromote).toHaveBeenCalledExactlyOnceWith(
					MockChatQueuedMessage.id,
				);
			} else {
				expect(onPromote).not.toHaveBeenCalled();
			}
			expect(onSend).not.toHaveBeenCalled();
		},
	);

	it.each([
		{ image: false, isEditingHistoryMessage: false },
		{ image: true, isEditingHistoryMessage: true },
	])(
		"isolates preview Escape and removes the attachment (image: $image, editing: $isEditingHistoryMessage)",
		async ({ image, isEditingHistoryMessage }) => {
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			const file = createMockFile(
				image ? "screenshot.png" : "notes.txt",
				image ? "image/png" : "text/plain",
			);
			const content = "Local attachment notes";
			const onRemoveAttachment = vi.fn();
			const onInterrupt = vi.fn();
			const onCancelHistoryEdit = vi.fn();
			renderInput(
				<ChatComposer
					{...inputProps}
					bindings={{
						...inputProps.bindings,
						inputRef,
						isStreaming: true,
						isEditingHistoryMessage,
						onInterrupt,
						onCancelHistoryEdit,
						files: {
							...inputProps.bindings.files,
							attachments: [file],
							textContents: new Map([[file, content]]),
							previewUrls: new Map([
								[file, "data:image/png;base64,iVBORw0KGgo="],
							]),
							onRemoveAttachment,
						},
					}}
				/>,
			);

			await user.click(
				screen.getByRole("button", {
					name: image ? "screenshot.png" : "View notes.txt",
				}),
			);
			await user.click(
				await screen.findByRole("dialog", {
					name: image ? "Image preview" : "notes.txt",
				}),
			);
			await user.keyboard("{Escape}");
			expect(onInterrupt).not.toHaveBeenCalled();
			expect(onCancelHistoryEdit).not.toHaveBeenCalled();

			if (image) {
				await user.click(
					screen.getByRole("button", { name: "Remove screenshot.png" }),
				);
			} else {
				await user.click(
					await screen.findByRole("button", { name: "Paste inline" }),
				);
				await waitFor(() =>
					expect(inputRef.current?.getValue()).toContain(content),
				);
			}
			expect(onRemoveAttachment).toHaveBeenCalledExactlyOnceWith(file);
		},
	);

	it("retains the loading composer draft on Enter even with a populated model", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onContentChange = vi.fn();

		renderInput(
			<LoadingChatComposer
				bindings={{
					inputRef,
					initialValue: "",
					onContentChange,
					isDisabled: false,
				}}
				model={inputProps.model}
				tools={inputProps.tools}
			/>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Draft while chat loads");
		await waitFor(() => {
			expect(onContentChange).toHaveBeenLastCalledWith(
				"Draft while chat loads",
				expect.any(String),
				false,
			);
		});
		await user.keyboard("{Enter}");
		expect(inputRef.current?.getValue()).toBe("Draft while chat loads");
	});

	it.each([
		{
			isDisabled: true,
			isStreaming: false,
			isLoading: false,
			isEditingHistoryMessage: false,
			key: "{Enter}",
		},
		{
			isDisabled: false,
			isStreaming: true,
			isLoading: false,
			isEditingHistoryMessage: true,
			key: "{Enter}",
		},
		{
			isDisabled: false,
			isStreaming: false,
			isLoading: true,
			isEditingHistoryMessage: true,
			key: "{Escape}",
		},
	])(
		"preserves drafts without submitting or canceling while locked (%o)",
		async ({
			isDisabled,
			isStreaming,
			isLoading,
			isEditingHistoryMessage,
			key,
		}) => {
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			const onSend = vi.fn();
			const onCancelHistoryEdit = vi.fn();

			renderInput(
				<ChatComposer
					{...inputProps}
					bindings={{
						...inputProps.bindings,
						onSend,
						inputRef,
						isDisabled,
						isStreaming,
						isLoading,
						isEditingHistoryMessage,
						onCancelHistoryEdit,
						initialValue: isLoading ? "Existing draft" : "",
					}}
				/>,
			);

			await user.click(screen.getByRole("textbox", { name: "Chat message" }));
			if (!isLoading) {
				await user.paste("Existing draft");
			}
			await waitFor(() => {
				expect(inputRef.current?.getValue()).toBe("Existing draft");
			});
			await user.keyboard(key);
			expect(onSend).not.toHaveBeenCalled();
			expect(onCancelHistoryEdit).not.toHaveBeenCalled();
			expect(inputRef.current?.getValue()).toBe("Existing draft");
		},
	);

	it.each([false, true])(
		"restores typing after send completion only on desktop (mobile: %s)",
		async (isMobile) => {
			stubViewport(isMobile);
			const user = userEvent.setup();
			const inputRef = createRef<ChatMessageInputRef>();
			const completion = createDeferred<undefined>();
			const onSend = vi.fn(() => completion.promise);
			const composer = (isLoading: boolean) => (
				<>
					<ChatComposer
						{...inputProps}
						bindings={{
							...inputProps.bindings,
							inputRef,
							onSend,
							isLoading,
							initialValue: "Draft",
						}}
					/>
					<input aria-label="Another input" />
				</>
			);
			const { rerender } = renderInput(composer(false));

			await user.click(screen.getByRole("button", { name: "Send" }));
			expect(onSend).toHaveBeenCalledExactlyOnceWith("Draft");
			rerender(composer(true));
			const anotherInput = screen.getByRole("textbox", {
				name: "Another input",
			});
			await user.click(anotherInput);

			await act(async () => completion.resolve(undefined));
			await user.keyboard("waiting");
			expect(anotherInput).toHaveValue("waiting");
			await act(async () => rerender(composer(false)));

			await user.paste(" next");
			await waitFor(() => {
				expect(inputRef.current?.getValue()).toBe(
					isMobile ? "Draft" : "Draft next",
				);
			});
			expect(anotherInput).toHaveValue(isMobile ? "waiting next" : "waiting");
		},
	);

	it("cycles prompt history and restores the draft without interrupting streaming", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const onInterrupt = vi.fn();
		const draft = "   ";
		renderInput(
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					inputRef,
					onSend,
					onInterrupt,
					isStreaming: true,
					initialValue: draft,
					userPromptHistory: ["Latest prompt", "Older prompt"],
				}}
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
		expect(onInterrupt).toHaveBeenCalledExactlyOnceWith();
	});

	it("discards the history session when the editor remounts", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const onInterrupt = vi.fn();
		const props = {
			...inputProps,
			bindings: {
				...inputProps.bindings,
				inputRef,
				onSend,
				onInterrupt,
				isStreaming: true,
				initialValue: "   ",
				userPromptHistory: ["Latest prompt", "Older prompt"],
			},
		};
		const { rerender } = renderInput(
			<StrictMode>
				<ChatComposer
					{...props}
					bindings={{ ...props.bindings, remountKey: 0 }}
				/>
			</StrictMode>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{ArrowUp}");
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("Latest prompt"),
		);

		rerender(
			<StrictMode>
				<ChatComposer
					{...props}
					bindings={{
						...props.bindings,
						remountKey: 1,
						initialValue: "Replacement draft",
					}}
				/>
			</StrictMode>,
		);
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("Replacement draft"),
		);
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{ArrowDown}{Escape}");
		expect(onInterrupt).toHaveBeenCalledExactlyOnceWith();
		await user.click(screen.getByRole("button", { name: "Queue" }));
		expect(onSend).toHaveBeenCalledExactlyOnceWith("Replacement draft");
	});

	it("keeps a retained editor handle live after remount", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const props = {
			...inputProps,
			bindings: { ...inputProps.bindings, inputRef, onSend },
		};
		const { rerender } = renderInput(<ChatComposer {...props} />);
		const handle = inputRef.current;

		rerender(
			<ChatComposer
				{...props}
				bindings={{ ...props.bindings, remountKey: 1 }}
			/>,
		);
		act(() => handle?.setValue("Replacement draft"));
		await waitFor(() => expect(handle?.getValue()).toBe("Replacement draft"));
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(onSend).toHaveBeenCalledExactlyOnceWith("Replacement draft");
	});

	it("attaches chat files and asks for a running workspace for other dropped files", () => {
		const onAttach = vi.fn();
		const toastError = vi.spyOn(toast, "error");

		renderInput(
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					files: { ...inputProps.bindings.files, onAttach },
				}}
			/>,
		);

		const svg = createMockFile("diagram.svg", "image/svg+xml");
		const zip = createMockFile("archive.zip", "application/zip");
		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: { files: [svg, zip] },
		});

		expect(onAttach).toHaveBeenCalledWith([svg]);
		expect(toastError).toHaveBeenCalledWith(
			"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.",
		);
	});

	it("removes a selected MCP server from the group", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<ChatComposer
				{...inputProps}
				tools={{
					...inputProps.tools,
					mcp: {
						servers: mockMCPServers,
						selectedServerIds: mockSelectedMCPServerIds,
						onSelectionChange: onMCPSelectionChange,
					},
				}}
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
			<ChatComposer
				{...inputProps}
				tools={{
					...inputProps.tools,
					mcp: {
						servers: [...mockMCPServers, mockNotionMCP],
						selectedServerIds: mockSelectedMCPServerIds,
						onSelectionChange: onMCPSelectionChange,
					},
				}}
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
			<ChatComposer
				{...inputProps}
				tools={{
					...inputProps.tools,
					mcp: {
						servers: [mockLinearMCP, mockGitHubMCP],
						selectedServerIds: [mockLinearMCP.id, mockGitHubMCP.id],
						onSelectionChange: onMCPSelectionChange,
					},
				}}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Remove Linear" }));
		expect(onMCPSelectionChange).toHaveBeenCalledWith([mockGitHubMCP.id]);
	});

	it("excludes selected MCP servers that still need OAuth from the group", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<ChatComposer
				{...inputProps}
				tools={{
					...inputProps.tools,
					mcp: {
						servers: [mockSentryMCP, mockLinearMCP, mockGitHubMCPNeedingAuth],
						selectedServerIds: [
							mockSentryMCP.id,
							mockLinearMCP.id,
							mockGitHubMCPNeedingAuth.id,
						],
						onSelectionChange: onMCPSelectionChange,
					},
				}}
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
			<ChatComposer
				{...inputProps}
				bindings={{ ...inputProps.bindings, isDisabled: true }}
				tools={{
					...inputProps.tools,
					mcp: {
						servers: mockMCPServers,
						selectedServerIds: mockSelectedMCPServerIds,
						onSelectionChange: onMCPSelectionChange,
					},
				}}
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					onSend,
					inputRef,
					isStreaming: true,
					onInterrupt,
				}}
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					onSend,
					isLoading: true,
					isStreaming: true,
					isInterruptPending: true,
					onInterrupt: vi.fn(),
					initialValue: "Also update the docs",
				}}
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
			...inputProps,
			bindings: {
				...inputProps.bindings,
				onSend: vi.fn(),
				isStreaming: true,
				onInterrupt: vi.fn(),
				initialValue: "Also update the docs",
			},
		};

		stubViewport(false);
		const desktop = renderInput(<ChatComposer {...props} />);
		expect(
			screen
				.getByRole("button", { name: "Queue" })
				.getAttribute("aria-keyshortcuts"),
		).toMatch(/Enter/);
		desktop.unmount();

		stubViewport(true);
		renderInput(<ChatComposer {...props} />);
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					inputRef,
					onSend,
					isStreaming: true,
					onInterrupt,
				}}
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

	it.each([
		{ isDisabled: false, isLoading: false, isReadOnly: false, starts: true },
		{ isDisabled: true, isLoading: false, isReadOnly: false, starts: true },
		{ isDisabled: false, isLoading: true, isReadOnly: false, starts: false },
		{ isDisabled: false, isLoading: false, isReadOnly: true, starts: false },
	])(
		"guards voice input while streaming (%o)",
		async ({ isDisabled, isLoading, isReadOnly, starts }) => {
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
				<ChatComposer
					{...inputProps}
					bindings={{
						...inputProps.bindings,
						isStreaming: true,
						isDisabled,
						isLoading,
						isReadOnly,
						onInterrupt: vi.fn(),
					}}
				/>,
			);

			await user.click(screen.getByRole("button", { name: "Voice input" }));
			expect(start).toHaveBeenCalledTimes(starts ? 1 : 0);
		},
	);

	it("falls back to pasted text when every pasted file is refused", async () => {
		const onAttach = vi.fn();
		const inputRef = createRef<ChatMessageInputRef>();

		renderInput(
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					inputRef,
					files: { ...inputProps.bindings.files, onAttach },
				}}
			/>,
		);

		const target = screen.getByRole("textbox", { name: "Chat message" });
		target.focus();
		// Without a running workspace, insert the clipboard text instead.
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

	it.each<{
		status: Exclude<WorkspaceFileUpload["status"], "uploaded">;
		deferred: boolean;
		sends: boolean;
	}>([
		{ status: "deferred", deferred: true, sends: true },
		{ status: "uploading", deferred: false, sends: false },
		{ status: "error", deferred: false, sends: false },
		{ status: "error", deferred: true, sends: true },
	])(
		"gates file-only submission on workspace readiness (%o)",
		async ({ status, deferred, sends }) => {
			const user = userEvent.setup();
			const onSend = vi.fn();
			const file = createMockFile("dataset.zip", "application/zip");
			const uploads: WorkspaceFileUpload[] =
				status === "error"
					? [{ id: "workspace-file", file, status, error: "Upload failed" }]
					: [{ id: "workspace-file", file, status }];
			renderInput(
				<ChatComposer
					{...inputProps}
					bindings={{
						...inputProps.bindings,
						onSend,
						files: {
							...inputProps.bindings.files,
							workspaceUploads: { uploads, onRemove: vi.fn(), deferred },
						},
					}}
				/>,
			);
			await user.click(screen.getByRole("textbox", { name: "Chat message" }));
			await user.keyboard("{Enter}");
			expect(onSend).toHaveBeenCalledTimes(sends ? 1 : 0);
			if (sends) {
				expect(onSend).toHaveBeenCalledWith("");
			}
		},
	);

	it("routes workspace files to workspace uploads instead of attachments", () => {
		const onAttach = vi.fn();
		const onWorkspaceAttach = vi.fn();

		renderInput(
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					files: {
						...inputProps.bindings.files,
						onAttach,
						workspaceUploads: {
							uploads: [],
							onAttach: onWorkspaceAttach,
							onRemove: vi.fn(),
						},
					},
				}}
			/>,
		);

		const zip = createMockFile("dataset.zip", "application/zip");
		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: { files: [zip] },
		});

		expect(onWorkspaceAttach).toHaveBeenCalledWith([zip]);
		expect(onAttach).not.toHaveBeenCalled();
	});

	it.each([
		{
			name: "workspace files while the composer is disabled",
			isDisabled: true,
			isLoading: false,
			paste: false,
			message:
				"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.",
		},
		{
			name: "dropped chat files while a send is pending",
			isDisabled: false,
			isLoading: true,
			paste: false,
			chatFile: true,
			message:
				"Wait for the current message to finish sending, then add the file again.",
		},
		{
			name: "pasted chat files while a send is pending",
			isDisabled: false,
			isLoading: true,
			paste: true,
			chatFile: true,
			message:
				"Wait for the current message to finish sending, then add the file again.",
		},
		{
			name: "all dropped files while read-only",
			isDisabled: false,
			isLoading: false,
			isReadOnly: true,
			paste: false,
			chatFile: true,
			message: undefined,
		},
	])(
		"refuses $name without losing the draft",
		async ({
			isDisabled,
			isLoading,
			isReadOnly = false,
			paste,
			chatFile = false,
			message,
		}) => {
			const inputRef = createRef<ChatMessageInputRef>();
			const onAttach = vi.fn();
			const onWorkspaceAttach = vi.fn();
			const toastError = vi.spyOn(toast, "error").mockClear();

			renderInput(
				<ChatComposer
					{...inputProps}
					bindings={{
						...inputProps.bindings,
						files: {
							...inputProps.bindings.files,
							onAttach,
							workspaceUploads: {
								uploads: [],
								onAttach: onWorkspaceAttach,
								onRemove: vi.fn(),
							},
						},
						isDisabled,
						isLoading,
						isReadOnly,
						inputRef,
						initialValue: "Existing draft",
					}}
				/>,
			);

			await waitFor(() =>
				expect(inputRef.current?.getValue()).toBe("Existing draft"),
			);
			const textbox = screen.getByRole("textbox", { name: "Chat message" });
			const files = [createMockFile("dataset.zip", "application/zip")];
			if (chatFile) {
				files.push(createMockFile("notes.txt", "text/plain"));
			}
			if (paste) {
				fireEvent.paste(textbox, {
					clipboardData: { files, types: ["Files"], getData: () => "" },
				});
			} else {
				expect(fireEvent.drop(textbox, { dataTransfer: { files } })).toBe(
					false,
				);
			}

			expect(onWorkspaceAttach).not.toHaveBeenCalled();
			expect(onAttach).not.toHaveBeenCalled();
			let expectedToasts: string[][] = [];

			if (message !== undefined) {
				expectedToasts = [[message]];

				if (paste) {
					expectedToasts = files.map(() => [message]);
				}
			}

			expect(toastError.mock.calls).toEqual(expectedToasts);
			expect(inputRef.current?.getValue()).toBe("Existing draft");
		},
	);
});
