import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Chat, ChatStatus } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { AssistantOpener, ChatOpener } from "./Openers";

const handlers = {
	onOpen: () => undefined,
	onPreview: () => undefined,
	onPreviewEnd: () => undefined,
};

const unread = (status: ChatStatus): Chat => ({
	...MockChat,
	has_unread: true,
	status,
});

describe("Openers", () => {
	it("shows the unread dot on an idle chat and hides it while the chat runs", () => {
		const { rerender } = renderComponent(
			<ChatOpener chat={unread("waiting")} {...handlers} />,
		);
		expect(screen.getByRole("img", { name: "Unread" })).toBeInTheDocument();

		rerender(<ChatOpener chat={unread("running")} {...handlers} />);
		expect(screen.queryByRole("img", { name: "Unread" })).toBeNull();
	});

	it.each([
		["waiting", "Assistant"],
		["running", "Assistant working"],
		["interrupting", "Assistant stopping"],
	] as const)("names a %s assistant %s", (status, name) => {
		renderComponent(
			<AssistantOpener assistant={{ ...MockChat, status }} {...handlers} />,
		);
		expect(screen.getByRole("button", { name })).toBeInTheDocument();
	});
});
