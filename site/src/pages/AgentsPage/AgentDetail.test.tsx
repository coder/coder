import type { ChatModelsResponse } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { MockWorkspace, MockWorkspaceAgent } from "testHelpers/entities";
import { renderComponent } from "testHelpers/renderHelpers";
import type { Mock } from "vitest";
import { AgentDetail } from "./AgentDetail";

const {
	mockGetApiKey,
	mockWatchChat,
	mockNavigate,
	mockUseParams,
	mockUseOutletContext,
	mockDisplayError,
} = vi.hoisted(() => ({
	mockGetApiKey: vi.fn(),
	mockWatchChat: vi.fn(),
	mockNavigate: vi.fn(),
	mockUseParams: vi.fn(),
	mockUseOutletContext: vi.fn(),
	mockDisplayError: vi.fn(),
}));

vi.mock("api/api", async () => {
	const actual = await vi.importActual<typeof import("api/api")>("api/api");
	return {
		...actual,
		API: {
			...actual.API,
			getApiKey: mockGetApiKey,
		},
		watchChat: mockWatchChat,
	};
});

vi.mock("react-router", async () => {
	const actual =
		await vi.importActual<typeof import("react-router")>("react-router");
	return {
		...actual,
		useNavigate: () => mockNavigate,
		useParams: () => mockUseParams(),
		useOutletContext: () => mockUseOutletContext(),
		Link: ({
			to,
			children,
			...props
		}: {
			to?: string;
			children?: ReactNode;
			[key: string]: unknown;
		}) => (
			<a href={typeof to === "string" ? to : "#"} {...props}>
				{children}
			</a>
		),
	};
});

vi.mock("react-query", async () => {
	const actual =
		await vi.importActual<typeof import("react-query")>("react-query");
	return {
		...actual,
		useQuery: vi.fn(),
		useMutation: vi.fn(),
		useQueryClient: vi.fn(),
	};
});

vi.mock("components/GlobalSnackbar/utils", () => ({
	displayError: mockDisplayError,
}));

vi.mock("./AgentChatInput", () => ({
	AgentChatInput: () => <div data-testid="agent-chat-input" />,
}));

vi.mock("./FilesChangedPanel", () => ({
	FilesChangedPanel: ({ chatId }: { chatId: string }) => (
		<div data-testid="files-changed-panel">{chatId}</div>
	),
}));

const mockUseQuery = useQuery as unknown as Mock;
const mockUseMutation = useMutation as unknown as Mock;
const mockUseQueryClient = useQueryClient as unknown as Mock;

const makeQueryResult = (overrides: Record<string, unknown> = {}) => ({
	data: undefined,
	error: null,
	isLoading: false,
	isError: false,
	...overrides,
});

type AgentDetailTestState = {
	readonly chatData: TypesGen.ChatWithMessages;
	readonly workspaceData: TypesGen.Workspace;
	readonly diffUrl?: string;
	readonly modelCatalog?: ChatModelsResponse;
};

type MockSocket = {
	addEventListener: ReturnType<typeof vi.fn>;
	removeEventListener: ReturnType<typeof vi.fn>;
	close: ReturnType<typeof vi.fn>;
	emitMessage: (payload: unknown) => void;
	emitError: (payload?: unknown) => void;
};

const createMockSocket = (): MockSocket => {
	const messageListeners = new Set<(payload: unknown) => void>();
	const errorListeners = new Set<(payload: unknown) => void>();

	return {
		addEventListener: vi.fn((type: string, listener: (payload: unknown) => void) => {
			if (type === "message") {
				messageListeners.add(listener);
				return;
			}
			if (type === "error") {
				errorListeners.add(listener);
			}
		}),
		removeEventListener: vi.fn(
			(type: string, listener: (payload: unknown) => void) => {
				if (type === "message") {
					messageListeners.delete(listener);
					return;
				}
				if (type === "error") {
					errorListeners.delete(listener);
				}
			},
		),
		close: vi.fn(),
		emitMessage: (payload: unknown) => {
			for (const listener of messageListeners) {
				listener(payload);
			}
		},
		emitError: (payload?: unknown) => {
			for (const listener of errorListeners) {
				listener(payload);
			}
		},
	};
};

const installQueryMocks = ({
	chatData,
	workspaceData,
	diffUrl,
	modelCatalog,
}: AgentDetailTestState) => {
	mockUseQuery.mockImplementation(
		(query: { queryKey?: readonly unknown[] } | undefined) => {
			const queryKey = query?.queryKey ?? [];
			if (queryKey[0] === "chat" && queryKey[2] === "diff-status") {
				return makeQueryResult({
					data: {
						chat_id: chatData.chat.id,
						url: diffUrl,
						changes_requested: false,
						additions: 4,
						deletions: 1,
						changed_files: 2,
					},
				});
			}
			if (queryKey[0] === "chat" && queryKey.length === 2) {
				return makeQueryResult({ data: chatData });
			}
			if (queryKey[0] === "workspace") {
				return makeQueryResult({ data: workspaceData });
			}
			if (queryKey[0] === "chat-models") {
				return makeQueryResult({
					data:
						modelCatalog ?? {
							providers: [
								{
									provider: "openai",
									available: true,
									models: [
										{
											id: "openai:gpt-4o",
											provider: "openai",
											model: "gpt-4o",
											display_name: "GPT-4o",
										},
									],
								},
							],
						},
				});
			}
			return makeQueryResult();
		},
	);
};

describe(AgentDetail.name, () => {
	beforeEach(() => {
		mockUseQuery.mockReset();
		mockUseMutation.mockReset();
		mockUseQueryClient.mockReset();
		mockGetApiKey.mockReset();
		mockWatchChat.mockReset();
		mockNavigate.mockReset();
		mockUseParams.mockReset();
		mockUseOutletContext.mockReset();
		mockDisplayError.mockReset();

		mockUseParams.mockReturnValue({ agentId: "chat-1" });
		mockUseMutation.mockImplementation(() => ({
			isPending: false,
			mutateAsync: vi.fn().mockResolvedValue(undefined),
		}));
		mockUseQueryClient.mockReturnValue({
			setQueryData: vi.fn(),
			invalidateQueries: vi.fn().mockResolvedValue(undefined),
		});
		mockGetApiKey.mockRejectedValue(new Error("missing API key"));
		mockWatchChat.mockReturnValue({
			addEventListener: vi.fn(),
			removeEventListener: vi.fn(),
			close: vi.fn(),
		});
	});

	it("renders actions menu, wires actions, and portals diff panel to the right slot", async () => {
		const topBarTitleRoot = document.createElement("div");
		const topBarActionsRoot = document.createElement("div");
		const rightPanelRoot = document.createElement("div");
		document.body.append(topBarTitleRoot, topBarActionsRoot, rightPanelRoot);

		const workspaceAgent = {
			...MockWorkspaceAgent,
			id: "workspace-agent-1",
			name: "workspace-agent",
			expanded_directory: "/workspace/project",
			apps: [],
		};
		const workspace = {
			...MockWorkspace,
			id: "workspace-1",
			owner_name: "owner",
			name: "workspace-name",
			latest_build: {
				...MockWorkspace.latest_build,
				resources: [
					{
						...MockWorkspace.latest_build.resources[0],
						agents: [workspaceAgent],
					},
				],
			},
		};
		const chatData: TypesGen.ChatWithMessages = {
			chat: {
				id: "chat-1",
				owner_id: "owner-id",
				workspace_id: workspace.id,
				workspace_agent_id: workspaceAgent.id,
				title: "Build a feature",
				status: "completed",
				model_config: {
					model: "gpt-4o",
					provider: "openai",
				},
				created_at: "2026-02-18T00:00:00.000Z",
				updated_at: "2026-02-18T00:00:00.000Z",
			},
			messages: [],
			queued_messages: [],
		};
		const requestArchiveAgent = vi.fn();
		const setRightPanelOpen = vi.fn();
		mockUseOutletContext.mockReturnValue({
			chatErrorReasons: {},
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
			topBarTitleRef: { current: topBarTitleRoot },
			topBarActionsRef: { current: topBarActionsRoot },
			rightPanelRef: { current: rightPanelRoot },
			setRightPanelOpen,
			requestArchiveAgent,
		});
		installQueryMocks({
			chatData,
			workspaceData: workspace,
			diffUrl: "https://github.com/coder/coder/pull/123",
		});

		renderComponent(<AgentDetail />);

		await waitFor(() => {
			expect(setRightPanelOpen).toHaveBeenCalledWith(true);
		});
		expect(within(rightPanelRoot).getByTestId("files-changed-panel")).toHaveTextContent(
			"chat-1",
		);

		const user = userEvent.setup();
		const menuTrigger = within(topBarActionsRoot).getByRole("button", {
			name: "Open agent actions",
		});

		await user.click(menuTrigger);
		expect(await screen.findByText("Open in Cursor")).toBeInTheDocument();
		expect(screen.getByText("Open in VS Code")).toBeInTheDocument();
		expect(screen.getByText("View Workspace")).toBeInTheDocument();
		expect(screen.getByText("Archive Agent")).toBeInTheDocument();

		await user.click(screen.getByText("Open in VS Code"));
		await waitFor(() => {
			expect(mockGetApiKey).toHaveBeenCalledTimes(1);
		});
		expect(mockDisplayError).toHaveBeenCalledWith("Failed to open in VS Code.");

		await user.click(menuTrigger);
		await user.click(screen.getByText("Open in Cursor"));
		await waitFor(() => {
			expect(mockGetApiKey).toHaveBeenCalledTimes(2);
		});
		expect(mockDisplayError).toHaveBeenCalledWith("Failed to open in Cursor.");

		await user.click(menuTrigger);
		await user.click(screen.getByText("View Workspace"));
		expect(mockNavigate).toHaveBeenCalledWith("/@owner/workspace-name");

		await user.click(menuTrigger);
		await user.click(screen.getByText("Archive Agent"));
		expect(requestArchiveAgent).toHaveBeenCalledWith("chat-1");
		topBarTitleRoot.remove();
		topBarActionsRoot.remove();
		rightPanelRoot.remove();
	});

	it("keeps diff panel closed when no diff status URL exists", async () => {
		const topBarTitleRoot = document.createElement("div");
		const topBarActionsRoot = document.createElement("div");
		const rightPanelRoot = document.createElement("div");
		document.body.append(topBarTitleRoot, topBarActionsRoot, rightPanelRoot);

		const workspace = {
			...MockWorkspace,
			id: "workspace-1",
		};
		const chatData: TypesGen.ChatWithMessages = {
			chat: {
				id: "chat-1",
				owner_id: "owner-id",
				workspace_id: workspace.id,
				workspace_agent_id: MockWorkspaceAgent.id,
				title: "No diff yet",
				status: "completed",
				model_config: {
					model: "gpt-4o",
					provider: "openai",
				},
				created_at: "2026-02-18T00:00:00.000Z",
				updated_at: "2026-02-18T00:00:00.000Z",
			},
			messages: [],
			queued_messages: [],
		};
		const setRightPanelOpen = vi.fn();
		mockUseOutletContext.mockReturnValue({
			chatErrorReasons: {},
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
			topBarTitleRef: { current: topBarTitleRoot },
			topBarActionsRef: { current: topBarActionsRoot },
			rightPanelRef: { current: rightPanelRoot },
			setRightPanelOpen,
			requestArchiveAgent: vi.fn(),
		});
		installQueryMocks({
			chatData,
			workspaceData: workspace,
			diffUrl: undefined,
		});

		renderComponent(<AgentDetail />);

		await waitFor(() => {
			expect(setRightPanelOpen).toHaveBeenCalledWith(false);
		});
		expect(within(rightPanelRoot).queryByTestId("files-changed-panel")).toBeNull();

		topBarTitleRoot.remove();
		topBarActionsRoot.remove();
		rightPanelRoot.remove();
	});

	it("applies child status events to subagent cards without changing parent status", async () => {
		const socket = createMockSocket();
		mockWatchChat.mockReturnValue(socket);

		const setQueryData = vi.fn();
		const invalidateQueries = vi.fn().mockResolvedValue(undefined);
		mockUseQueryClient.mockReturnValue({
			setQueryData,
			invalidateQueries,
		});

		const workspace = {
			...MockWorkspace,
			id: "workspace-1",
		};
		const chatData: TypesGen.ChatWithMessages = {
			chat: {
				id: "chat-1",
				owner_id: "owner-id",
				workspace_id: workspace.id,
				workspace_agent_id: MockWorkspaceAgent.id,
				title: "Parent agent",
				status: "running",
				model_config: {
					model: "gpt-4o",
					provider: "openai",
				},
				created_at: "2026-02-18T00:00:00.000Z",
				updated_at: "2026-02-18T00:00:00.000Z",
			},
			messages: [
				{
					id: 1,
					chat_id: "chat-1",
					created_at: "2026-02-18T00:00:01.000Z",
					role: "assistant",
					hidden: false,
					parts: [
						{
							type: "tool-call",
							tool_call_id: "tool-subagent-1",
							tool_name: "subagent",
							args: {
								title: "Child agent",
							},
						},
						{
							type: "tool-result",
							tool_call_id: "tool-subagent-1",
							tool_name: "subagent",
							result: {
								chat_id: "child-chat-1",
								title: "Child agent",
								status: "pending",
							},
						},
					],
				},
			],
			queued_messages: [],
		};

		mockUseOutletContext.mockReturnValue({
			chatErrorReasons: {},
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
			topBarTitleRef: { current: null },
			topBarActionsRef: { current: null },
			rightPanelRef: { current: null },
			setRightPanelOpen: vi.fn(),
			requestArchiveAgent: vi.fn(),
		});
		installQueryMocks({
			chatData,
			workspaceData: workspace,
			diffUrl: undefined,
		});

		renderComponent(<AgentDetail />);

		expect(screen.getByText("Thinking...")).toBeInTheDocument();
		expect(
			screen.getByRole("button", { name: /Spawning Child agent/ }),
		).toBeInTheDocument();

		socket.emitMessage({
			parseError: null,
			parsedMessage: {
				type: "data",
				data: {
					type: "status",
					chat_id: "child-chat-1",
					status: {
						status: "completed",
					},
				},
			},
		});

		await waitFor(() => {
			expect(
				screen.getByRole("button", { name: /Spawned Child agent/ }),
			).toBeInTheDocument();
		});
		expect(screen.getByText("Thinking...")).toBeInTheDocument();
		expect(setQueryData).not.toHaveBeenCalled();
		expect(invalidateQueries).not.toHaveBeenCalled();
	});

	it("renders streamed subagent title before args JSON is complete", async () => {
		const socket = createMockSocket();
		mockWatchChat.mockReturnValue(socket);

		const workspace = {
			...MockWorkspace,
			id: "workspace-1",
		};
		const chatData: TypesGen.ChatWithMessages = {
			chat: {
				id: "chat-1",
				owner_id: "owner-id",
				workspace_id: workspace.id,
				workspace_agent_id: MockWorkspaceAgent.id,
				title: "Streaming title",
				status: "running",
				model_config: {
					model: "gpt-4o",
					provider: "openai",
				},
				created_at: "2026-02-18T00:00:00.000Z",
				updated_at: "2026-02-18T00:00:00.000Z",
			},
			messages: [],
			queued_messages: [],
		};

		mockUseOutletContext.mockReturnValue({
			chatErrorReasons: {},
			setChatErrorReason: vi.fn(),
			clearChatErrorReason: vi.fn(),
			topBarTitleRef: { current: null },
			topBarActionsRef: { current: null },
			rightPanelRef: { current: null },
			setRightPanelOpen: vi.fn(),
			requestArchiveAgent: vi.fn(),
		});
		installQueryMocks({
			chatData,
			workspaceData: workspace,
			diffUrl: undefined,
		});

		renderComponent(<AgentDetail />);

		socket.emitMessage({
			parseError: null,
			parsedMessage: {
				type: "data",
				data: {
					type: "message_part",
					message_part: {
						part: {
							type: "tool-call",
							tool_call_id: "tool-subagent-stream-1",
							tool_name: "subagent",
							args_delta: "{\"title\":\"Streamed Child\"",
						},
					},
				},
			},
		});

		await waitFor(() => {
			expect(
				screen.getByRole("button", { name: /Spawning Streamed Child/ }),
			).toBeInTheDocument();
		});

		socket.emitMessage({
			parseError: null,
			parsedMessage: {
				type: "data",
				data: {
					type: "message_part",
					message_part: {
						part: {
							type: "tool-call",
							tool_call_id: "tool-subagent-stream-1",
							tool_name: "subagent",
							args_delta: ",\"prompt\":\"Finish setup\"}",
						},
					},
				},
			},
		});

		await waitFor(() => {
			expect(
				screen.getByRole("button", { name: /Spawning Streamed Child/ }),
			).toBeInTheDocument();
		});
	});
});
