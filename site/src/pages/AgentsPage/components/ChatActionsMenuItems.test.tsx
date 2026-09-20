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
import {
	MockChatTreeChild,
	MockChatTreeRoot,
	MockChatTreeSubagent,
} from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	ChatActionsMenuItems,
	chatHasMenuActions,
} from "./ChatActionsMenuItems";

const renderMenu = (
	chat: Chat,
	options: { isParentArchived?: boolean } = {},
) => {
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
					isParentArchived={options.isParentArchived}
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
	screen.queryAllByRole("menuitem").map((item) => item.textContent?.trim());

describe("ChatActionsMenuItems", () => {
	it("keeps pin and archive on a named child chat", async () => {
		const handlers = renderMenu(MockChatTreeChild);
		await userEvent.click(screen.getByRole("menuitem", { name: "Pin agent" }));
		await userEvent.click(
			screen.getByRole("menuitem", { name: "Archive agent" }),
		);
		expect(handlers.onPinAgent).toHaveBeenCalledTimes(1);
		expect(handlers.onArchiveAgent).toHaveBeenCalledTimes(1);
	});

	it("offers only rename on the tree root", () => {
		renderMenu(MockChatTreeRoot);
		expect(menuItemNames()).toEqual(["Rename chat"]);
	});

	it("offers only rename on a subagent", () => {
		renderMenu(MockChatTreeSubagent);
		expect(menuItemNames()).toEqual(["Rename chat"]);
	});

	it("hides unarchive on an archived child whose parent is archived", () => {
		renderMenu(
			{ ...MockChatTreeChild, archived: true },
			{
				isParentArchived: true,
			},
		);
		expect(menuItemNames()).not.toContain("Unarchive agent");
	});
});

describe(chatHasMenuActions.name, () => {
	it("hides the menu only for archived subagents", () => {
		expect(
			chatHasMenuActions({ ...MockChatTreeSubagent, archived: true }),
		).toBe(false);
		expect(chatHasMenuActions({ ...MockChatTreeChild, archived: true })).toBe(
			true,
		);
		expect(chatHasMenuActions({ ...MockChatTreeRoot, archived: false })).toBe(
			true,
		);
	});
});
