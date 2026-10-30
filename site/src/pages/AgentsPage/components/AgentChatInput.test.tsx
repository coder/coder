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
import { createMemoryRouter, MemoryRouter, useLocation } from "react-router";
import { afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { AppProviders } from "#/App";
import { API } from "#/api/api";
import {
	organizationSkillPreferencesKey,
	organizationSkills,
} from "#/api/queries/organizationSkills";
import { userSkills } from "#/api/queries/userSkills";
import type * as TypesGen from "#/api/typesGenerated";
import { toast } from "#/components/Toaster/toast";
import { MockMCPServerConfig } from "#/testHelpers/chatEntities";
import { MockChatProject, MockWorkspace } from "#/testHelpers/entities";
import { createMockFile } from "#/testHelpers/files";
import {
	createTestQueryClient,
	renderWithRouter,
} from "#/testHelpers/renderHelpers";
import {
	MockOrganizationSkillPreferences,
	MockSkill,
} from "#/testHelpers/skills";
import {
	belowMdViewportMediaQuery,
	mobileViewportMediaQuery,
} from "#/utils/mobile";
import type * as speechRecognition from "../hooks/useSpeechRecognition";
import { useSpeechRecognition } from "../hooks/useSpeechRecognition";
import { AgentChatInput, type ChatMessageInputRef } from "./AgentChatInput";

vi.mock("#/modules/dashboard/useDashboard", () => ({
	useDashboard: () => ({
		organizations: [],
	}),
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

const stubViewport = (
	matches: boolean,
	mediaQuery = mobileViewportMediaQuery,
) => {
	vi.stubGlobal("matchMedia", (query: string) => {
		const result = originalMatchMedia(query);
		return query === mediaQuery ? { ...result, matches } : result;
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
	return render(<AppProviders>{children}</AppProviders>, {
		wrapper: MemoryRouter,
	});
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
	vi.restoreAllMocks();
});

describe("AgentChatInput", () => {
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

	it("offers the chat organization's skills in the slash menu", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const queryClient = createTestQueryClient();
		queryClient.setQueryData(userSkills().queryKey, []);
		queryClient.setQueryData(organizationSkills("org-1").queryKey, [
			{ ...MockSkill, name: "release-notes" },
		]);
		queryClient.setQueryData(
			organizationSkillPreferencesKey("org-1"),
			MockOrganizationSkillPreferences,
		);

		render(
			<AppProviders queryClient={queryClient}>
				<AgentChatInput
					{...inputProps}
					inputRef={inputRef}
					chatOrganizationId="org-1"
				/>
			</AppProviders>,
			{ wrapper: MemoryRouter },
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("/rel");
		await user.click(
			await screen.findByRole("option", { name: /release-notes/ }),
		);
		expect(inputRef.current?.getContentParts()).toContainEqual({
			type: "skill-invocation",
			source: "org",
			name: "release-notes",
		});
	});

	it("sends a message that holds only a skill chip", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onSend = vi.fn();
		renderInput(
			<AgentChatInput {...inputProps} onSend={onSend} inputRef={inputRef} />,
		);
		await waitFor(() => expect(inputRef.current).not.toBeNull());

		act(() => inputRef.current?.insertSkill("project", "triage"));
		await user.click(screen.getByRole("button", { name: "Send" }));
		expect(onSend).toHaveBeenCalledTimes(1);
	});

	it("offers the chat project's skills in the slash menu", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		vi.spyOn(API.experimental, "getUserSkills").mockResolvedValue([]);
		vi.spyOn(API.experimental, "getOrganizationSkills").mockResolvedValue([]);
		vi.spyOn(
			API.experimental,
			"getOrganizationSkillPreferences",
		).mockResolvedValue(MockOrganizationSkillPreferences);
		const getChatProjectSkills = vi
			.spyOn(API.experimental, "getChatProjectSkills")
			.mockResolvedValue([{ ...MockSkill, name: "project-conventions" }]);
		vi.spyOn(API.experimental, "getChatProjects").mockResolvedValue([
			{ ...MockChatProject, id: "project-1" },
		]);

		render(
			<AppProviders queryClient={createTestQueryClient()}>
				<AgentChatInput
					{...inputProps}
					inputRef={inputRef}
					chatOrganizationId="org-1"
					chatProjectId="project-1"
				/>
			</AppProviders>,
			{ wrapper: MemoryRouter },
		);

		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("/proj");
		await user.click(
			await screen.findByRole("option", { name: /project-conventions/ }),
		);
		expect(inputRef.current?.getContentParts()).toContainEqual({
			type: "skill-invocation",
			source: "project",
			name: "project-conventions",
		});
		expect(getChatProjectSkills).toHaveBeenCalledWith("org-1", "project-1");
	});

	it.each([
		["a pending send settles", { isLoading: true }],
		["the composer is enabled", { isDisabled: true }],
	])("loads project actions only once %s", async (_, blocked) => {
		const getChatProjectSkills = vi
			.spyOn(API.experimental, "getChatProjectSkills")
			.mockResolvedValue([]);
		const queryClient = createTestQueryClient();
		const renderInput = (isBlocked: boolean) => (
			<AppProviders queryClient={queryClient}>
				<AgentChatInput
					{...inputProps}
					{...(isBlocked ? blocked : {})}
					chatOrganizationId="org-1"
					chatProjectId="project-1"
					showProjectActions
				/>
			</AppProviders>
		);

		const { rerender } = render(renderInput(true), { wrapper: MemoryRouter });
		await screen.findByRole("textbox", { name: "Chat message" });
		expect(getChatProjectSkills).not.toHaveBeenCalled();

		rerender(renderInput(false));
		await waitFor(() =>
			expect(getChatProjectSkills).toHaveBeenCalledWith("org-1", "project-1"),
		);
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

	it("turns a server off from the group pill's menu", async () => {
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
			within(screen.getByRole("dialog")).getByRole("switch", {
				name: "Disable Linear",
			}),
		);
		expect(onMCPSelectionChange).toHaveBeenCalledWith([
			mockSentryMCP.id,
			mockGitHubMCP.id,
		]);
	});

	it("keeps keyboard focus in the menu through the below-md submenus", async () => {
		const user = userEvent.setup();
		stubViewport(true, belowMdViewportMediaQuery);
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={mockMCPServers}
				selectedMCPServerIds={mockSelectedMCPServerIds}
				onMCPSelectionChange={vi.fn()}
				workspaceOptions={[MockWorkspace]}
				onWorkspaceChange={vi.fn()}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "More options" }));
		(await screen.findByRole("button", { name: /MCP servers/ })).focus();
		await user.keyboard("{Enter}");
		expect(await screen.findByRole("button", { name: "Back" })).toHaveFocus();
		await user.keyboard("{Enter}");
		expect(screen.getByRole("button", { name: /MCP servers/ })).toHaveFocus();

		screen.getByRole("button", { name: "Attach workspace" }).focus();
		await user.keyboard("{Enter}");
		expect(await screen.findByRole("button", { name: "Back" })).toHaveFocus();
		await user.keyboard("{Enter}");
		expect(
			screen.getByRole("button", { name: "Attach workspace" }),
		).toHaveFocus();
	});

	it("enables an unselected MCP server from the MCP servers submenu", async () => {
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
			await screen.findByRole("button", { name: /MCP servers/ }),
		);
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

	it("announces a disconnected always-on server as always on", async () => {
		const user = userEvent.setup();
		const mockDisconnectedSentryMCP = {
			...mockSentryMCP,
			auth_connected: false,
		};
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[mockDisconnectedSentryMCP, mockLinearMCP]}
				selectedMCPServerIds={[mockDisconnectedSentryMCP.id, mockLinearMCP.id]}
				onMCPSelectionChange={vi.fn()}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(
			await screen.findByRole("button", { name: /MCP servers/ }),
		);
		const menu = await screen.findByRole("group", { name: "MCP servers" });
		expect(menu).toHaveTextContent(/Sentry\s*Always on/);
		expect(
			within(menu).getByRole("img", { name: "Not connected" }),
		).toBeInTheDocument();
	});

	it("turns a server on from the empty MCP pill", async () => {
		const user = userEvent.setup();
		const onMCPSelectionChange = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[mockLinearMCP, mockGitHubMCP]}
				selectedMCPServerIds={[]}
				onMCPSelectionChange={onMCPSelectionChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Add MCPs" }));
		await user.click(
			within(screen.getByRole("dialog")).getByRole("switch", {
				name: "Enable Linear",
			}),
		);
		expect(onMCPSelectionChange).toHaveBeenCalledWith([mockLinearMCP.id]);
	});

	it("excludes selected MCP servers that still need OAuth from the pill count", () => {
		renderInput(
			<AgentChatInput
				{...inputProps}
				mcpServers={[mockSentryMCP, mockLinearMCP, mockGitHubMCPNeedingAuth]}
				selectedMCPServerIds={[
					mockSentryMCP.id,
					mockLinearMCP.id,
					mockGitHubMCPNeedingAuth.id,
				]}
				onMCPSelectionChange={vi.fn()}
			/>,
		);

		expect(screen.getByRole("button", { name: "2 MCPs" })).toBeInTheDocument();
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
			within(screen.getByRole("dialog")).getByRole("switch", {
				name: "Disable Linear",
			}),
		);
		expect(onMCPSelectionChange).not.toHaveBeenCalled();
	});

	it("opens MCP connections settings with a way back to the chat", async () => {
		const user = userEvent.setup();
		const onSettingsState = vi.fn();
		const SettingsPage = () => {
			onSettingsState(useLocation().state);
			return null;
		};
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/agents/:chatId",
						element: (
							<AgentChatInput
								{...inputProps}
								chatOrganizationId="org-1"
								mcpServers={[mockLinearMCP]}
								selectedMCPServerIds={[]}
								onMCPSelectionChange={vi.fn()}
							/>
						),
					},
					{
						path: "/agents/settings/mcp-connections",
						element: <SettingsPage />,
					},
				],
				{ initialEntries: ["/agents/chat-1?tab=git"] },
			),
		);

		await user.click(screen.getByRole("button", { name: "More options" }));
		await user.click(
			await screen.findByRole("button", { name: /MCP servers/ }),
		);
		await user.click(
			await screen.findByRole("button", { name: "Manage MCP connections" }),
		);
		await waitFor(() =>
			expect(onSettingsState).toHaveBeenCalledWith({
				from: "/agents/chat-1?tab=git",
				organizationId: "org-1",
			}),
		);
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
				initialValue=""
				onContentChange={vi.fn()}
				planModeEnabled={false}
				onPlanModeToggle={vi.fn()}
				isModelCatalogLoading={false}
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

	it("lets Escape on a chip's remove button cancel a history edit", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const onCancelHistoryEdit = vi.fn();
		renderInput(
			<AgentChatInput
				{...inputProps}
				inputRef={inputRef}
				isEditingHistoryMessage
				onCancelHistoryEdit={onCancelHistoryEdit}
			/>,
		);
		await waitFor(() => expect(inputRef.current).not.toBeNull());
		act(() => inputRef.current?.insertSkill("project", "triage"));
		const remove = await screen.findByRole("button", {
			name: "Remove skill /project/triage",
		});
		act(() => remove.focus());

		await user.keyboard("{Escape}");
		expect(onCancelHistoryEdit).toHaveBeenCalledTimes(1);
	});

	it("keeps skill chips through a voice recording and its cancellation", async () => {
		const user = userEvent.setup();
		const inputRef = createRef<ChatMessageInputRef>();
		const speech = { isRecording: false, transcript: "" };
		mockedUseSpeechRecognition.mockImplementation(() => ({
			isSupported: true,
			error: null,
			...speech,
			start: vi.fn(),
			stop: vi.fn(),
			cancel: vi.fn(),
		}));
		const input = () => (
			<AppProviders>
				<AgentChatInput {...inputProps} inputRef={inputRef} />
			</AppProviders>
		);
		const { rerender } = render(input(), { wrapper: MemoryRouter });
		await waitFor(() => expect(inputRef.current).not.toBeNull());
		act(() => inputRef.current?.insertSkill("project", "triage"));
		const chip = {
			type: "skill-invocation",
			source: "project",
			name: "triage",
		};

		await user.click(screen.getByRole("button", { name: "Voice input" }));
		Object.assign(speech, { isRecording: true, transcript: "check this" });
		rerender(input());
		await waitFor(() =>
			expect(inputRef.current?.getContentParts()).toEqual([
				chip,
				{ type: "text", text: " check this" },
			]),
		);
		await user.click(
			screen.getByRole("button", { name: "Cancel voice input" }),
		);
		Object.assign(speech, { isRecording: false, transcript: "" });
		rerender(input());

		await waitFor(() =>
			expect(inputRef.current?.getContentParts()).toEqual([
				chip,
				{ type: "text", text: " " },
			]),
		);
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
