import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { toast } from "sonner";
import { afterEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockWorkspace, MockWorkspaceAgent } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { createChatStore } from "./ChatConversation/chatStore";
import { ChatPageInput } from "./ChatPageContent";

const renderChatPageInput = (
	store: ReturnType<typeof createChatStore>,
	overrides: Partial<ComponentProps<typeof ChatPageInput>> = {},
) =>
	renderWithAuth(
		<ChatPageInput
			chat={{ ...MockChat, id: "", organization_id: "" }}
			store={store}
			models={[]}
			onSend={vi.fn()}
			onDeleteQueuedMessage={vi.fn()}
			onPromoteQueuedMessage={vi.fn()}
			onInterrupt={vi.fn()}
			isInputDisabled={false}
			isSendPending={false}
			isInterruptPending={false}
			hasModelOptions
			selectedModel="model-config-1"
			onModelChange={vi.fn()}
			modelOptions={[
				{
					id: "model-config-1",
					provider: "openai",
					model: "gpt-4o",
					displayName: "GPT-4o",
				},
			]}
			modelSelectorPlaceholder="Select model"
			canConfigureAgentSetup={false}
			isEditing={false}
			onCancelHistoryEdit={vi.fn()}
			{...overrides}
		/>,
	);

const workspaceFileReference = (
	name: string,
	workspaceId: string,
): TypesGen.ChatWorkspaceFileReferencePart => ({
	type: "workspace-file-reference",
	workspace_file_path: `/home/coder/${name}`,
	workspace_file_name: name,
	workspace_file_size: 42,
	workspace_file_workspace_id: workspaceId,
	workspace_file_media_type: "text/csv",
});

const mockMainAgent: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "agent-main",
	name: "main",
	status: "connected",
};

const mockChatAgent: TypesGen.WorkspaceAgent = {
	...MockWorkspaceAgent,
	id: "agent-chat",
	name: "dev-coderd-chat",
	status: "disconnected",
};

const mockMultiAgentWorkspace: TypesGen.Workspace = {
	...MockWorkspace,
	id: "ws-multi",
	latest_build: {
		...MockWorkspace.latest_build,
		resources: [
			{
				...MockWorkspace.latest_build.resources[0],
				agents: [mockMainAgent, mockChatAgent],
			},
		],
	},
};

const attachZipFile = async (user: ReturnType<typeof userEvent.setup>) => {
	const zip = new File([new Uint8Array([0x50, 0x4b, 3, 4])], "bundle.zip", {
		type: "application/zip",
	});
	await user.upload(
		await screen.findByTestId("chat-attachment-file-input"),
		zip,
	);
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe("ChatPageInput", () => {
	it("routes Stop to onInterrupt while the chat requires action", async () => {
		const user = userEvent.setup();
		const onInterrupt = vi.fn();
		const store = createChatStore();
		store.setChatStatus("requires_action");

		renderChatPageInput(store, { onInterrupt });

		await user.click(await screen.findByRole("button", { name: "Stop" }));
		expect(onInterrupt).toHaveBeenCalledTimes(1);
	});

	it("rehydrates edited workspace file references only from the bound workspace", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn();

		renderChatPageInput(createChatStore(), {
			chat: {
				...MockChat,
				id: "",
				organization_id: "",
				workspace_id: "ws-1",
			},
			onSend,
			isEditing: true,
			initialValue: "edited",
			editingFileBlocks: [
				workspaceFileReference("current.csv", "ws-1"),
				workspaceFileReference("other.csv", "ws-2"),
				workspaceFileReference("unbound.csv", ""),
			],
		});

		await user.click(await screen.findByRole("button", { name: "Save Edit" }));

		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		expect(onSend.mock.calls[0][0].workspaceUploads).toEqual([
			{
				path: "/home/coder/current.csv",
				name: "current.csv",
				size: 42,
				mediaType: "text/csv",
				workspaceId: "ws-1",
			},
		]);
	});

	it("freezes the composer's attachments while an edit is pending", async () => {
		const uploadChatFile = vi.spyOn(API.experimental, "uploadChatFile");
		const uploadChatWorkspaceFile = vi.spyOn(
			API.experimental,
			"uploadChatWorkspaceFile",
		);
		vi.spyOn(toast, "error");

		renderChatPageInput(createChatStore(), {
			chat: {
				...MockChat,
				organization_id: "",
				workspace_id: MockWorkspace.id,
				agent_id: MockWorkspaceAgent.id,
			},
			workspace: MockWorkspace,
			isEditing: true,
			isSendPending: true,
			initialValue: "edited",
			editingFileBlocks: [
				workspaceFileReference("current.csv", MockWorkspace.id),
			],
		});

		expect(
			await screen.findByRole("button", { name: "Remove current.csv" }),
		).toBeDisabled();
		fireEvent.drop(screen.getByRole("textbox", { name: "Chat message" }), {
			dataTransfer: {
				files: [
					new File(["png"], "screenshot.png", { type: "image/png" }),
					new File([new Uint8Array([0x50, 0x4b, 3, 4])], "bundle.zip", {
						type: "application/zip",
					}),
				],
			},
		});

		expect(toast.error).toHaveBeenCalledTimes(1);
		expect(toast.error).toHaveBeenCalledWith(
			"Wait for the current message to finish sending, then add the file again.",
		);
		expect(uploadChatFile).not.toHaveBeenCalled();
		expect(uploadChatWorkspaceFile).not.toHaveBeenCalled();
	});

	it.each([
		{ name: "a null", agentId: undefined },
		{ name: "a stale", agentId: "agent-removed" },
	])(
		"uses the server-selected agent for uploads with $name agent_id",
		async ({ agentId }) => {
			const user = userEvent.setup({ applyAccept: true });
			vi.spyOn(toast, "error");
			const getChatWorkspaceAgent = vi
				.spyOn(API.experimental, "getChatWorkspaceAgent")
				.mockResolvedValue({ agent_id: mockChatAgent.id });
			const uploadChatWorkspaceFile = vi.spyOn(
				API.experimental,
				"uploadChatWorkspaceFile",
			);

			renderChatPageInput(createChatStore(), {
				chat: {
					...MockChat,
					organization_id: "",
					workspace_id: mockMultiAgentWorkspace.id,
					agent_id: agentId,
				},
				workspace: mockMultiAgentWorkspace,
			});

			await waitFor(() =>
				expect(getChatWorkspaceAgent).toHaveBeenCalledWith(
					mockMultiAgentWorkspace.id,
				),
			);
			await attachZipFile(user);

			expect(toast.error).toHaveBeenCalledWith(
				"This file type is uploaded into the chat's workspace. Attach a running workspace to the chat, then try again.",
			);
			expect(uploadChatWorkspaceFile).not.toHaveBeenCalled();
		},
	);

	it("uploads to the server-selected agent when it is connected", async () => {
		const user = userEvent.setup({ applyAccept: true });
		vi.spyOn(API.experimental, "getChatWorkspaceAgent").mockResolvedValue({
			agent_id: mockMainAgent.id,
		});
		const uploadChatWorkspaceFile = vi
			.spyOn(API.experimental, "uploadChatWorkspaceFile")
			.mockResolvedValue({
				path: "/home/coder/bundle.zip",
				name: "bundle.zip",
				size: 4,
				media_type: "application/zip",
				workspace_id: mockMultiAgentWorkspace.id,
			});

		renderChatPageInput(createChatStore(), {
			chat: {
				...MockChat,
				organization_id: "",
				workspace_id: mockMultiAgentWorkspace.id,
				agent_id: undefined,
			},
			workspace: {
				...mockMultiAgentWorkspace,
				latest_build: {
					...mockMultiAgentWorkspace.latest_build,
					resources: [
						{
							...mockMultiAgentWorkspace.latest_build.resources[0],
							agents: [mockMainAgent, { ...mockChatAgent, name: "sidecar" }],
						},
					],
				},
			},
		});

		await waitFor(() =>
			expect(API.experimental.getChatWorkspaceAgent).toHaveBeenCalled(),
		);
		await attachZipFile(user);

		await waitFor(() => expect(uploadChatWorkspaceFile).toHaveBeenCalled());
	});
});
