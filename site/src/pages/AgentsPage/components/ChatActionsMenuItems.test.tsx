import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
} from "#/components/DropdownMenu/DropdownMenu";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	ChatActionsMenuItems,
	chatHasMenuActions,
} from "./ChatActionsMenuItems";

const MockNamedChildChat: Chat = {
	...MockChat,
	id: "child-chat-1",
	kind: "chat",
	parent_chat_id: "root-chat-1",
};

const MockRootChat: Chat = {
	...MockChat,
	id: "root-chat-1",
	kind: "root",
	title: "Root",
};

const MockSubagentChat: Chat = {
	...MockChat,
	id: "subagent-1",
	kind: "subagent",
	parent_chat_id: "chat-1",
};

const renderMenu = (chat: Chat) => {
	const handlers = {
		onPinAgent: vi.fn(),
		onUnpinAgent: vi.fn(),
		onArchiveAgent: vi.fn(),
		onUnarchiveAgent: vi.fn(),
		onArchiveAndDeleteWorkspace: vi.fn(),
		onOpenRenameDialog: vi.fn(),
	};
	renderComponent(
		<DropdownMenu open>
			<DropdownMenuContent>
				<ChatActionsMenuItems
					chat={chat}
					hasWorkspace
					{...handlers}
					Item={DropdownMenuItem}
					Separator={DropdownMenuSeparator}
				/>
			</DropdownMenuContent>
		</DropdownMenu>,
	);
	return handlers;
};

const menuItemNames = () =>
	screen.getAllByRole("menuitem").map((item) => item.textContent?.trim());

describe("ChatActionsMenuItems", () => {
	it("keeps pin and archive on a named child chat", async () => {
		const handlers = renderMenu(MockNamedChildChat);
		await userEvent.click(screen.getByRole("menuitem", { name: "Pin agent" }));
		await userEvent.click(
			screen.getByRole("menuitem", { name: "Archive agent" }),
		);
		expect(handlers.onPinAgent).toHaveBeenCalledTimes(1);
		expect(handlers.onArchiveAgent).toHaveBeenCalledTimes(1);
	});

	it("offers only rename on the tree root", () => {
		renderMenu(MockRootChat);
		expect(menuItemNames()).toEqual(["Rename chat"]);
	});

	it("offers only rename on a subagent", () => {
		renderMenu(MockSubagentChat);
		expect(menuItemNames()).toEqual(["Rename chat"]);
	});
});

describe(chatHasMenuActions.name, () => {
	it("hides the menu only for archived subagents", () => {
		expect(chatHasMenuActions({ ...MockSubagentChat, archived: true })).toBe(
			false,
		);
		expect(chatHasMenuActions({ ...MockNamedChildChat, archived: true })).toBe(
			true,
		);
		expect(chatHasMenuActions({ ...MockRootChat, archived: false })).toBe(true);
	});
});
