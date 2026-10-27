import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { Chat, ChatProject } from "#/api/typesGenerated";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockChatProject } from "#/testHelpers/entities";
import { renderWithRouter } from "#/testHelpers/renderHelpers";
import { ChatSearchDialog } from "./ChatSearchDialog";

const mockProjects: ChatProject[] = [
	{ ...MockChatProject, id: "project-1", name: "Agent evals" },
	{ ...MockChatProject, id: "project-2", name: "Fast builds" },
	{ ...MockChatProject, id: "project-3", name: "Fire dashboard" },
	{ ...MockChatProject, id: "project-4", name: "Launch" },
	{ ...MockChatProject, id: "project-5", name: "Release notes" },
];

const mockSearchChat: Chat = {
	...MockChat,
	id: "chat-search-result",
	title: "Fix the fire dashboard filters",
};

const SearchDialog: React.FC<{
	readonly onOpenChange: (open: boolean) => void;
}> = ({ onOpenChange }) => (
	<ChatSearchDialog
		open
		onOpenChange={onOpenChange}
		location={useLocation()}
		recentChats={[]}
		projects={mockProjects}
	/>
);

const renderDialog = () => {
	const onOpenChange = vi.fn();
	const { router } = renderWithRouter(
		createMemoryRouter(
			[
				{
					path: "/agents",
					element: <SearchDialog onOpenChange={onOpenChange} />,
				},
				{ path: "/agents/*", element: null },
			],
			{ initialEntries: ["/agents?view=grouped"] },
		),
	);
	return { router, onOpenChange };
};

const getSearchInput = () =>
	screen.getByRole("combobox", { name: "Search chats and projects" });

describe("ChatSearchDialog projects", () => {
	beforeEach(() => {
		vi.spyOn(API.experimental, "getChats").mockResolvedValue([mockSearchChat]);
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("opens a matching project with Enter", async () => {
		const user = userEvent.setup();
		const { router, onOpenChange } = renderDialog();

		await user.type(getSearchInput(), "dashboard");
		// The default view lists projects too, so wait for a search result.
		await screen.findByRole("option", { name: /Fix the fire dashboard/ });
		await user.keyboard("{ArrowDown}{Enter}");

		expect(router.state.location.pathname).toBe("/agents/projects/project-3");
		expect(router.state.location.search).toBe("?view=grouped");
		expect(onOpenChange).toHaveBeenCalledWith(false);
	});

	it("selects the chats after the matching projects", async () => {
		const user = userEvent.setup();
		const { router } = renderDialog();

		await user.type(getSearchInput(), "dashboard");
		await screen.findByRole("option", { name: /Fix the fire dashboard/ });
		await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");

		expect(router.state.location.pathname).toBe(`/agents/${mockSearchChat.id}`);
	});

	it("does not offer projects while a filter is active", async () => {
		const user = userEvent.setup();
		const { router } = renderDialog();

		await user.type(getSearchInput(), "has_unread:true dashboard");
		await waitFor(() => {
			expect(API.experimental.getChats).toHaveBeenCalledWith(
				expect.objectContaining({
					q: 'has_unread:true search:"dashboard"',
				}),
				expect.any(AbortSignal),
			);
		});
		await screen.findByRole("option", { name: /Fix the fire dashboard/ });
		await user.keyboard("{ArrowDown}{Enter}");

		expect(router.state.location.pathname).toBe(`/agents/${mockSearchChat.id}`);
	});

	it("keeps matching projects selectable when the chat search fails", async () => {
		vi.spyOn(API.experimental, "getChats").mockRejectedValue(
			new Error("Bad filter"),
		);
		const user = userEvent.setup();
		const { router } = renderDialog();

		await user.type(getSearchInput(), "dashboard");
		await screen.findByRole("alert");
		await user.keyboard("{ArrowDown}{Enter}");

		expect(router.state.location.pathname).toBe("/agents/projects/project-3");
	});

	it("reveals the hidden projects from the Show all option", async () => {
		const user = userEvent.setup();
		const { router } = renderDialog();

		// Three projects come first, so the fourth option is "Show all".
		await user.click(getSearchInput());
		await user.keyboard("{ArrowDown>4/}{Enter}");
		// The expanded list puts the fourth project at the same position.
		await user.keyboard("{Enter}");

		expect(router.state.location.pathname).toBe("/agents/projects/project-4");
	});

	it("keeps focus in the input after clicking Show all", async () => {
		const user = userEvent.setup();
		const { router } = renderDialog();

		await user.click(
			screen.getByRole("option", { name: "Show all 5 projects" }),
		);
		await user.keyboard("{ArrowUp}{Enter}");

		expect(router.state.location.pathname).toBe("/agents/projects/project-5");
	});
});
