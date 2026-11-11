import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import type { ChatProjectSummary } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectSummary,
} from "#/testHelpers/entities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import { ProjectCard } from "./ProjectCard";

const renderCard = (overrides: Partial<ChatProjectSummary> = {}) => {
	const callbacks = {
		onPin: vi.fn(),
		onUnpin: vi.fn(),
		onEdit: vi.fn(),
		onDelete: vi.fn(),
	};
	renderComponent(
		<MemoryRouter>
			<ProjectCard
				summary={{ ...MockChatProjectSummary, ...overrides }}
				to="/agents/projects/chat-project-1"
				{...callbacks}
			/>
		</MemoryRouter>,
	);
	return callbacks;
};

describe("ProjectCard", () => {
	// The card link's description is what a screen reader announces for the
	// card's status.
	it.each<[string, Partial<ChatProjectSummary>, RegExp]>([
		[
			"a chat needing attention over a running one",
			{ own_chat_count: 3, own_needs_attention: true, own_running: true },
			/^Needs attention/,
		],
		[
			"a running chat over unread chats",
			{ own_chat_count: 3, own_unread_count: 1, own_running: true },
			/^Thinking/,
		],
		["unread chats", { own_chat_count: 3, own_unread_count: 1 }, /^1 unread/],
		["read chats", { own_chat_count: 3 }, /^3 read/],
		["no chats", { own_chat_count: 0 }, /^No chats/],
		["a private project", { member_count: 1 }, /Only you$/],
		["a shared project", { member_count: 1234 }, /1,234 members$/],
	])("describes %s", (_, overrides, description) => {
		renderCard(overrides);
		expect(
			screen.getByRole("link", { name: MockChatProject.name }),
		).toHaveAccessibleDescription(description);
	});

	// Unpinning removes the pin button, which must not drop focus to the page.
	it("unpins from the pin button and keeps focus in the card", async () => {
		const user = userEvent.setup();
		const { onUnpin } = renderCard();
		await user.click(
			screen.getByRole("button", { name: `Unpin ${MockChatProject.name}` }),
		);
		expect(onUnpin).toHaveBeenCalledOnce();
		expect(
			screen.getByRole("button", {
				name: `Open project actions for ${MockChatProject.name}`,
			}),
		).toHaveFocus();
	});

	it("pins from the actions menu with the keyboard", async () => {
		const user = userEvent.setup();
		const { onPin } = renderCard({ pinned: false, pin_order: 0 });
		// The link, then the actions button.
		await user.tab();
		await user.tab();
		await user.keyboard("{Enter}");
		await screen.findByRole("menuitem", { name: "Pin project" });
		await user.keyboard("{Enter}");
		expect(onPin).toHaveBeenCalledOnce();
	});
});
