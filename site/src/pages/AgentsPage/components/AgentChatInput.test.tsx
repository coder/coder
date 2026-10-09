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
import {
	AgentComposer,
	AgentComposerProvider,
	type ComposerContextValue,
	useAgentComposer,
} from "./AgentComposer";
import { ChatComposer } from "./AgentComposers";
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
		hasModelOptions: true,
		initialValue: "",
		onContentChange: vi.fn(),
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

describe("ChatComposer", () => {
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
			<AgentComposerProvider
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
			</AgentComposerProvider>,
		);
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Shared draft");
		await user.click(screen.getByRole("button", { name: "Submit draft" }));
		expect(onSend).toHaveBeenCalledExactlyOnceWith("Shared draft");
	});

	it("supports an injected public contract without the default runtime", async () => {
		const user = userEvent.setup();
		const editorRef = createRef<ChatMessageInputRef>();
		const fileInputRef = createRef<HTMLInputElement>();
		const onSubmit = vi.fn();
		const onFiles = vi.fn();
		const value: ComposerContextValue = {
			state: {
				isDisabled: false,
				isReadOnly: false,
				isLoading: false,
				isStreaming: false,
				isInterruptPending: false,
				isEditingHistoryMessage: false,
				isDragging: false,
				invisibleCharCount: 0,
				canSend: true,
				showSendButton: true,
				showStopButton: false,
				canAttachFiles: true,
				speechSupported: false,
				speechRecording: false,
				speechError: null,
			},
			actions: {
				openFilePicker: () => fileInputRef.current?.click(),
				resetPromptCycle: vi.fn(),
				submit: () => onSubmit(editorRef.current?.getValue()),
				startRecording: vi.fn(),
				acceptRecording: vi.fn(),
				cancelRecording: vi.fn(),
				fileSelect: (event) => onFiles(Array.from(event.target.files ?? [])),
				filePaste: vi.fn(() => true),
				inlineText: vi.fn(),
				textPreview: vi.fn(),
				imagePreview: vi.fn(),
				contentChange: vi.fn(),
				editorKeyDown: vi.fn(),
				composerKeyDown: vi.fn(),
				dragOver: vi.fn(),
				dragLeave: vi.fn(),
				drop: vi.fn(),
			},
			meta: {
				attachEditor: vi.fn((editor) => {
					editorRef.current = editor;
				}),
				attachFileInput: vi.fn((input) => {
					fileInputRef.current = input;
				}),
				warningId: "injected-composer-warning",
				composerElement: null,
				setComposerElement: vi.fn(),
				initialValue: "",
				sendShortcut: "enter",
				sendShortcutLabel: undefined,
				attachments: [],
			},
		};
		const SiblingActions = () => {
			const { actions } = useAgentComposer();
			return (
				<>
					<button type="button" onClick={actions.openFilePicker}>
						Attach draft file
					</button>
					<button type="button" onClick={actions.submit}>
						Submit draft
					</button>
				</>
			);
		};
		const children = (
			<>
				<AgentComposer.Frame>
					<AgentComposer.Attachments />
					<AgentComposer.Editor hasWorkspace={false} />
					<AgentComposer.Submit />
				</AgentComposer.Frame>
				<SiblingActions />
			</>
		);
		const { rerender } = renderInput(
			<AgentComposer.Provider {...value}>{children}</AgentComposer.Provider>,
		);
		const editor = editorRef.current;
		const fileInput = fileInputRef.current;
		if (!fileInput || !editor) {
			throw new Error("Expected injected editor and file input handles");
		}
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("Injected draft");
		await waitFor(() =>
			expect(value.actions.contentChange).toHaveBeenLastCalledWith(
				"Injected draft",
				expect.any(String),
				false,
			),
		);
		const file = createMockFile("notes.txt", "text/plain");
		await user.upload(fileInput, file);
		expect(onFiles).toHaveBeenCalledExactlyOnceWith([file]);
		const click = vi.spyOn(fileInput, "click");
		await user.click(screen.getByRole("button", { name: "Attach draft file" }));
		expect(click).toHaveBeenCalledExactlyOnceWith();
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(onSubmit).toHaveBeenCalledExactlyOnceWith("Injected draft");

		rerender(
			<AppProviders>
				<AgentComposer.Provider
					{...value}
					state={{ ...value.state, warning: "Updated warning" }}
				>
					{children}
				</AgentComposer.Provider>
			</AppProviders>,
		);
		expect(editorRef.current).toBe(editor);
		expect(fileInputRef.current).toBe(fileInput);
		expect(value.meta.attachEditor).toHaveBeenCalledTimes(1);
		expect(value.meta.attachFileInput).toHaveBeenCalledTimes(1);
		await user.click(screen.getByRole("button", { name: "Submit draft" }));
		expect(onSubmit).toHaveBeenNthCalledWith(2, "Injected draft");
	});

	it.each([
		{ isDisabled: false, isLoading: false, promoted: true },
		{ isDisabled: true, isLoading: false, promoted: false },
		{ isDisabled: false, isLoading: true, promoted: false },
	])(
		"handles empty Enter with the canonical queue (%o)",
		async ({ isDisabled, isLoading, promoted }) => {
			const user = userEvent.setup();
			const onSend = vi.fn();
			const onPromote = vi.fn();
			renderInput(
				<ChatComposer
					{...inputProps}
					bindings={{ ...inputProps.bindings, onSend, isDisabled, isLoading }}
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

	it("inlines locally previewed text and removes its attachment", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const file = createMockFile("notes.txt", "text/plain");
		const content = "Local attachment notes";
		const onRemoveAttachment = vi.fn();
		renderInput(
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					inputRef,
					attachments: [file],
					textContents: new Map([[file, content]]),
					onRemoveAttachment,
				}}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "View notes.txt" }));
		await user.click(await screen.findByRole("dialog", { name: "notes.txt" }));
		await user.keyboard("{Escape}");
		await user.click(
			await screen.findByRole("button", { name: "Paste inline" }),
		);
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toContain(content),
		);

		expect(onRemoveAttachment).toHaveBeenCalledExactlyOnceWith(file);
	});

	it("keeps the editor mounted while composing history-edit actions", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const props = {
			...inputProps,
			bindings: { ...inputProps.bindings, inputRef, onSend },
		};
		const { rerender } = renderInput(<ChatComposer {...props} />);
		const handle = inputRef.current;
		const textbox = screen.getByRole("textbox", { name: "Chat message" });

		await user.click(textbox);
		await user.paste("Preserved draft");
		await waitFor(() => expect(handle?.getValue()).toBe("Preserved draft"));

		rerender(
			<AppProviders>
				<ChatComposer
					{...props}
					bindings={{ ...props.bindings, isEditingHistoryMessage: true }}
				/>
			</AppProviders>,
		);

		expect(inputRef.current).toBe(handle);
		expect(screen.getByRole("textbox", { name: "Chat message" })).toBe(textbox);
		await user.click(screen.getByRole("button", { name: "Save Edit" }));
		expect(onSend).toHaveBeenCalledExactlyOnceWith("Preserved draft");

		rerender(
			<AppProviders>
				<ChatComposer {...props} />
			</AppProviders>,
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(onSend).toHaveBeenCalledTimes(2);
		expect(onSend).toHaveBeenLastCalledWith("Preserved draft");
	});

	it("fills its container when the page column sets the width", () => {
		localStorage.removeItem("agents.chat-full-width");
		const { rerender } = renderInput(<ChatComposer {...inputProps} />);
		const column = screen.getByTestId("chat-composer").parentElement;

		expect(column).toHaveClass("max-w-3xl");

		rerender(
			<AppProviders>
				<ChatComposer {...inputProps} fillWidth />
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					onSend,
					inputRef,
					isDisabled: true,
				}}
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

	it.each([
		{ isMobile: false, rejects: false, completesWhileLoading: false },
		{ isMobile: false, rejects: true, completesWhileLoading: false },
		{ isMobile: false, rejects: false, completesWhileLoading: true },
		{ isMobile: false, rejects: true, completesWhileLoading: true },
		{ isMobile: true, rejects: false, completesWhileLoading: true },
		{ isMobile: true, rejects: true, completesWhileLoading: true },
	])(
		"restores typing only on desktop after submission completes (mobile: $isMobile, rejects: $rejects, completes while loading: $completesWhileLoading)",
		async ({ isMobile, rejects, completesWhileLoading }) => {
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
			rerender(<AppProviders>{composer(true)}</AppProviders>);
			const anotherInput = screen.getByRole("textbox", {
				name: "Another input",
			});
			await user.click(anotherInput);

			if (!completesWhileLoading) {
				await act(async () => {
					rerender(<AppProviders>{composer(false)}</AppProviders>);
				});
				await user.keyboard("waiting");
				expect(anotherInput).toHaveValue("waiting");
			}

			await act(async () => {
				if (rejects) {
					completion.reject(new Error("Send failed"));
				} else {
					completion.resolve(undefined);
				}
			});

			if (completesWhileLoading) {
				await user.keyboard("waiting");
				expect(anotherInput).toHaveValue("waiting");
				await act(async () => {
					rerender(<AppProviders>{composer(false)}</AppProviders>);
				});
			}

			await user.paste(" next");
			await waitFor(() => {
				expect(inputRef.current?.getValue()).toBe(
					isMobile ? "Draft" : "Draft next",
				);
			});
			expect(anotherInput).toHaveValue(isMobile ? "waiting next" : "waiting");
		},
	);

	it("does not move typing focus when loading ends without a submission", async () => {
		stubViewport(false);
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const composer = (isLoading: boolean) => (
			<>
				<ChatComposer
					{...inputProps}
					bindings={{ ...inputProps.bindings, inputRef, onSend, isLoading }}
				/>
				<input aria-label="Another input" />
			</>
		);
		const { rerender } = renderInput(composer(false));
		rerender(<AppProviders>{composer(true)}</AppProviders>);
		const anotherInput = screen.getByRole("textbox", { name: "Another input" });
		await user.click(anotherInput);
		rerender(<AppProviders>{composer(false)}</AppProviders>);

		await user.keyboard("Continue elsewhere");
		expect(anotherInput).toHaveValue("Continue elsewhere");
		expect(inputRef.current?.getValue()).toBe("");
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
				<ChatComposer
					{...inputProps}
					bindings={{
						...inputProps.bindings,
						inputRef,
						onSend,
						onInterrupt,
						isStreaming,
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
			expect(onInterrupt).toHaveBeenCalledTimes(isStreaming ? 1 : 0);
		},
	);

	it("ends prompt cycling when recalled text is edited", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const onInterrupt = vi.fn();

		renderInput(
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					inputRef,
					onSend,
					onInterrupt,
					isStreaming: true,
					userPromptHistory: ["Latest prompt", "Older prompt"],
				}}
			/>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{ArrowUp}");
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("Latest prompt"),
		);
		await user.keyboard(" edited");
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("Latest prompt edited"),
		);

		await user.keyboard("{ArrowUp}{ArrowDown}{Escape}");
		expect(onInterrupt).toHaveBeenCalledExactlyOnceWith();
		await user.click(screen.getByRole("button", { name: "Queue" }));
		expect(onSend).toHaveBeenCalledExactlyOnceWith("Latest prompt edited");
	});

	it("keeps the original history snapshot until cycling ends", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		const props = {
			...inputProps,
			bindings: { ...inputProps.bindings, inputRef, onSend },
		};
		const { rerender } = renderInput(
			<ChatComposer
				{...props}
				bindings={{
					...props.bindings,
					userPromptHistory: ["Latest prompt", "Older prompt"],
				}}
			/>,
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{ArrowUp}");
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("Latest prompt"),
		);

		rerender(
			<AppProviders>
				<ChatComposer
					{...props}
					bindings={{ ...props.bindings, userPromptHistory: ["New prompt"] }}
				/>
			</AppProviders>,
		);
		await user.keyboard("{ArrowUp}");
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("Older prompt"),
		);
		await user.keyboard("{Escape}{ArrowUp}");
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("New prompt"),
		);
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(onSend).toHaveBeenCalledExactlyOnceWith("New prompt");
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
			<AppProviders>
				<StrictMode>
					<ChatComposer
						{...props}
						bindings={{
							...props.bindings,
							remountKey: 1,
							initialValue: "Replacement draft",
						}}
					/>
				</StrictMode>
			</AppProviders>,
		);
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("Replacement draft"),
		);
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{ArrowDown}{Escape}");
		expect(onInterrupt).toHaveBeenCalledExactlyOnceWith();
		await user.click(screen.getByRole("button", { name: "Queue" }));
		expect(onSend).toHaveBeenCalledExactlyOnceWith("Replacement draft");

		const freshDraft = " ";
		rerender(
			<AppProviders>
				<StrictMode>
					<ChatComposer
						{...props}
						bindings={{
							...props.bindings,
							remountKey: 2,
							initialValue: freshDraft,
							userPromptHistory: ["New latest prompt", "New older prompt"],
						}}
					/>
				</StrictMode>
			</AppProviders>,
		);
		await waitFor(() => expect(inputRef.current?.getValue()).toBe(freshDraft));
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.keyboard("{ArrowDown}");
		expect(inputRef.current?.getValue()).toBe(freshDraft);
		await user.keyboard("{ArrowUp}");
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("New latest prompt"),
		);
		await user.keyboard("{ArrowUp}");
		await waitFor(() =>
			expect(inputRef.current?.getValue()).toBe("New older prompt"),
		);
		await user.keyboard("{ArrowDown}{ArrowDown}");
		await waitFor(() => expect(inputRef.current?.getValue()).toBe(freshDraft));
		await user.keyboard("{ArrowUp}{Escape}");
		await waitFor(() => expect(inputRef.current?.getValue()).toBe(freshDraft));
		expect(onInterrupt).toHaveBeenCalledExactlyOnceWith();
		expect(onSend).toHaveBeenCalledExactlyOnceWith("Replacement draft");
	});

	it("does not replace a non-empty draft with prompt history", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		renderInput(
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					inputRef,
					onSend,
					initialValue: "Unfinished draft",
					userPromptHistory: ["Latest prompt"],
				}}
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
			vi.fn<
				React.ComponentProps<typeof ChatComposer>["bindings"]["onContentChange"]
			>();
		const props = {
			...inputProps,
			bindings: { ...inputProps.bindings, inputRef, onSend, onContentChange },
		};
		const { rerender } = renderInput(
			<ChatComposer
				{...props}
				bindings={{ ...props.bindings, remountKey: 0 }}
			/>,
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
		if (!serializedState) {
			throw new Error("Expected serialized editor state");
		}
		act(() => handle?.clear());
		await waitFor(() => expect(handle?.getContentParts()).toEqual([]));

		rerender(
			<AppProviders>
				<ChatComposer
					{...props}
					bindings={{
						...props.bindings,
						remountKey: 1,
						initialValue: "Plain-text fallback",
						initialEditorState: serializedState,
					}}
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
			<ChatComposer
				{...inputProps}
				bindings={{ ...inputProps.bindings, onAttach, attachments: [] }}
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					isStreaming: true,
					onInterrupt: vi.fn(),
				}}
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					inputRef,
					onAttach,
					attachments: [],
				}}
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					onAttach,
					attachments: [],
					workspaceUploads: {
						uploads: [],
						onAttach: onWorkspaceAttach,
						onRemove: vi.fn(),
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

	it("refuses workspace files while the composer is disabled", () => {
		const onAttach = vi.fn();
		const onWorkspaceAttach = vi.fn();
		const toastError = vi.spyOn(toast, "error");

		renderInput(
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					onAttach,
					attachments: [],
					workspaceUploads: {
						uploads: [],
						onAttach: onWorkspaceAttach,
						onRemove: vi.fn(),
					},
					isDisabled: true,
				}}
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					onAttach,
					attachments: [],
					workspaceUploads: {
						uploads: [],
						onAttach: onWorkspaceAttach,
						onRemove: vi.fn(),
					},
					isLoading: true,
				}}
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					onAttach,
					attachments: [],
					workspaceUploads: {
						uploads: [],
						onAttach: onWorkspaceAttach,
						onRemove: vi.fn(),
					},
					isLoading: true,
				}}
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
			<ChatComposer
				{...inputProps}
				bindings={{
					...inputProps.bindings,
					onAttach,
					attachments: [],
					workspaceUploads: { uploads: [], onRemove: vi.fn() },
				}}
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
