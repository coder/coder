import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Chat } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { RenameChatDialog } from "./RenameChatDialog";

const renderDialog = (chat: Chat) => {
	const onRename = vi.fn(async () => {});
	renderComponent(
		<RenameChatDialog chat={chat} onRename={onRename} onOpenChange={vi.fn()} />,
	);
	return { onRename };
};

describe("RenameChatDialog", () => {
	it("submits the unchanged text of a fallback title", async () => {
		const user = userEvent.setup();
		const chat: Chat = {
			...MockChat,
			title: "derived from prompt",
			title_source: "fallback",
		};
		const { onRename } = renderDialog(chat);

		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onRename).toHaveBeenCalledWith(chat.id, "derived from prompt");
	});

	it("submits a user title only once it changes", async () => {
		const user = userEvent.setup();
		const chat: Chat = {
			...MockChat,
			title: "Chosen",
			title_source: "user",
		};
		const { onRename } = renderDialog(chat);

		await user.click(screen.getByRole("button", { name: "Save" }));
		expect(onRename).not.toHaveBeenCalled();

		const input = screen.getByRole("textbox", { name: "Chat title" });
		await user.clear(input);
		await user.type(input, "Chosen again");
		await user.click(screen.getByRole("button", { name: "Save" }));

		expect(onRename).toHaveBeenCalledWith(chat.id, "Chosen again");
	});
});
