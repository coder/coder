import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { beforeAll, describe, expect, it, vi } from "vitest";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import {
	getParkedWorkspaceUploads,
	unparkWorkspaceUploads,
} from "../utils/parkedWorkspaceUploads";
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

beforeAll(() => {
	Object.defineProperty(Range.prototype, "getBoundingClientRect", {
		configurable: true,
		value: () => new DOMRect(0, 0, 1, 16),
	});
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

	it("parks workspace files until the chat has a running workspace", async () => {
		const user = userEvent.setup();
		const onSend = vi.fn();
		const chatId = "chat-without-workspace";
		renderChatPageInput(createChatStore(), {
			chat: { ...MockChat, id: chatId, organization_id: "", workspace_id: "" },
			onSend,
		});

		await user.upload(
			await screen.findByTestId("chat-attachment-file-input"),
			new File([new Uint8Array(4)], "bundle.zip", { type: "application/zip" }),
		);
		await screen.findByText(
			"Uploads when the workspace starts. Keep this chat open.",
		);
		await user.click(screen.getByRole("textbox", { name: "Chat message" }));
		await user.paste("create a workspace and unpack the bundle");
		await user.click(screen.getByRole("button", { name: "Send" }));

		await waitFor(() => expect(onSend).toHaveBeenCalledTimes(1));
		expect(onSend.mock.calls[0][0]).toMatchObject({
			message: "create a workspace and unpack the bundle",
			workspaceUploads: undefined,
		});
		const parkedFiles = getParkedWorkspaceUploads(chatId).map(
			(upload) => upload.file,
		);
		expect(parkedFiles.map((file) => file.name)).toEqual(["bundle.zip"]);
		unparkWorkspaceUploads(chatId, parkedFiles);
	});
});
