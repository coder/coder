import { MessageScroller } from "@shadcn/react/message-scroller";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { AgentChatPageView } from "./AgentChatPageView";
import { createChatStore } from "./components/ChatConversation/chatStore";
import { lastActiveSidebarTabStorageKeyPrefix } from "./utils/sidebarTabStorage";

const chat = { ...MockChat, owner_id: MockUserOwner.id };
const storageKey = lastActiveSidebarTabStorageKeyPrefix + chat.id;
const onSetShowSidebarPanel = vi.fn();
const Page = ({ initiallyOpen = false }: { initiallyOpen?: boolean }) => {
	const [open, setOpen] = useState(initiallyOpen);
	const [store] = useState(createChatStore);
	return (
		<MessageScroller.Provider autoScroll defaultScrollPosition="end">
			<AgentChatPageView
				chat={chat}
				store={store}
				initialMessages={[]}
				persistedError={undefined}
				effectiveSelectedModel={chat.last_model_config_id}
				setSelectedModel={vi.fn()}
				modelOptions={[]}
				models={[]}
				mcpServers={[]}
				selectedMCPServerIds={[]}
				onMCPSelectionChange={vi.fn()}
				onMCPAuthComplete={vi.fn()}
				modelSelectorPlaceholder="Select model"
				hasModelOptions={false}
				canConfigureAgentSetup={false}
				isInputDisabled={false}
				isSubmissionPending={false}
				isInterruptPending={false}
				showSidebarPanel={open}
				onSetShowSidebarPanel={(next) => {
					onSetShowSidebarPanel(next);
					setOpen(next);
				}}
				debugLoggingEnabled={false}
				gitWatcher={{
					repositories: new Map(),
					everDirty: new Set(),
					hasReceivedChanges: true,
					refresh: vi.fn(),
				}}
				sshCommand={undefined}
				handleCommit={vi.fn()}
				handleInterrupt={vi.fn()}
				handleDeleteQueuedMessage={vi.fn()}
				handlePromoteQueuedMessage={vi.fn()}
				hasMoreMessages={false}
				isFetchingMoreMessages={false}
				isHydratingMessages={false}
				hasFetchMoreError={false}
				onFetchMoreMessages={vi.fn()}
				editing={{
					chatInputRef: { current: null },
					editorInitialValue: "",
					initialEditorState: undefined,
					remountKey: 0,
					editingMessageId: null,
					editingFileBlocks: [],
					handleEditUserMessage: vi.fn(),
					handleCancelHistoryEdit: vi.fn(),
					handleSendFromInput: vi.fn(),
					handleContentChange: vi.fn(),
				}}
			/>
		</MessageScroller.Provider>
	);
};

beforeEach(() => {
	localStorage.removeItem(storageKey);
	onSetShowSidebarPanel.mockClear();
	vi.spyOn(API.experimental, "getChat").mockResolvedValue(chat);
	vi.spyOn(API.experimental, "refreshChatContext").mockResolvedValue(chat);
});

describe("Details navigation", () => {
	it.each([false, true])(
		"opens and selects Details from Git without toggling it closed (open=%s)",
		async (initiallyOpen) => {
			localStorage.setItem(storageKey, "git");
			const user = userEvent.setup();
			renderWithAuth(<Page initiallyOpen={initiallyOpen} />);
			const trigger = await screen.findByRole("button", {
				name: /Context usage:/,
			});
			await user.click(trigger);
			await user.tab();
			expect(screen.getByRole("button", { name: "Details" })).toHaveFocus();
			await user.keyboard("{Enter}");
			await waitFor(() =>
				expect(screen.getByRole("region", { name: "Details" })).toHaveFocus(),
			);
			expect(onSetShowSidebarPanel).toHaveBeenLastCalledWith(true);
			expect(localStorage.getItem(storageKey)).toBe("summary");
			expect(API.experimental.refreshChatContext).not.toHaveBeenCalled();
			await user.click(trigger);
			await user.click(screen.getByRole("button", { name: "Details" }));
			await waitFor(() =>
				expect(screen.getByRole("region", { name: "Details" })).toHaveFocus(),
			);
			expect(onSetShowSidebarPanel).not.toHaveBeenCalledWith(false);
		},
	);
	it("restores the panel toggle when closing an initially open panel", async () => {
		const user = userEvent.setup();
		renderWithAuth(<Page initiallyOpen />);
		await user.click(
			await screen.findByRole("button", { name: "Close panel" }),
		);
		await waitFor(() =>
			expect(
				screen.getByRole("button", { name: "Toggle panel" }),
			).toHaveFocus(),
		);
		expect(onSetShowSidebarPanel).toHaveBeenLastCalledWith(false);
	});
	it("keeps the stored summary ID and restores its opener when closed", async () => {
		localStorage.setItem(storageKey, "summary");
		const user = userEvent.setup();
		renderWithAuth(<Page />);
		const trigger = await screen.findByRole("button", {
			name: /Context usage:/,
		});
		await user.click(trigger);
		await user.click(screen.getByRole("button", { name: "Details" }));
		await user.click(screen.getByRole("button", { name: "Close panel" }));
		await waitFor(() => expect(trigger).toHaveFocus());
		expect(localStorage.getItem(storageKey)).toBe("summary");
		expect(API.experimental.refreshChatContext).not.toHaveBeenCalled();
	});
});
