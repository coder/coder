import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProject, ChatProjectSummary } from "#/api/typesGenerated";
import { createDeferred } from "#/testHelpers/deferred";
import {
	MockArchivedChatProject,
	MockChatProject,
	MockChatProjectSummary,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import ProjectsPage from "./ProjectsPage";

// The header reads the agents layout's outlet context.
vi.mock("../components/AgentPageHeader", () => ({
	AgentPageHeader: () => null,
}));

const summary = (
	name: string,
	overrides: Partial<ChatProjectSummary> = {},
	ownerId = MockUserOwner.id,
): ChatProjectSummary => ({
	...MockChatProjectSummary,
	pinned: false,
	pin_order: 0,
	...overrides,
	project: { ...MockChatProject, id: name, name, owner_id: ownerId },
});

const cardNames = () =>
	screen.getAllByRole("link").map((link) => link.textContent);

describe("ProjectsPage", () => {
	it("orders your projects and lists others' projects under Shared with you", async () => {
		const user = userEvent.setup();
		const earlier = "2026-11-01T00:00:00Z";
		const later = "2026-11-02T00:00:00Z";
		vi.spyOn(API.experimental, "getChatProjectSummaries").mockResolvedValue([
			summary("zeta"),
			summary("Alpha"),
			summary("theirs", {}, MockUserMember.id),
			summary("older", { last_activity_at: earlier }),
			summary("second pin", { pinned: true, pin_order: 2 }),
			summary("newer", { last_activity_at: later }),
			summary("Tied", { last_activity_at: later }),
			summary("first pin", { pinned: true, pin_order: 1 }),
		]);
		renderWithAuth(<ProjectsPage />);
		await screen.findByRole("link", { name: "zeta" });
		expect(cardNames()).toEqual([
			"first pin",
			"second pin",
			"newer",
			"Tied",
			"older",
			"Alpha",
			"zeta",
		]);
		await user.click(screen.getByRole("tab", { name: "Shared with you" }));
		await waitFor(() => expect(cardNames()).toEqual(["theirs"]));
	});

	it("pins at the end and holds back Pin while each project's pin is pending", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChatProjectSummaries").mockResolvedValue([
			summary("India"),
			summary("Kilo"),
		]);
		const pin = vi
			.spyOn(API.experimental, "pinChatProject")
			.mockReturnValue(new Promise(() => {}));
		renderWithAuth(<ProjectsPage />);
		const openActions = async (name: string) =>
			user.click(
				await screen.findByRole("button", {
					name: `Open project actions for ${name}`,
				}),
			);

		// Pins run in order, so Kilo's pin waits behind India's.
		for (const name of ["India", "Kilo"]) {
			await openActions(name);
			await user.click(
				await screen.findByRole("menuitem", { name: "Pin project" }),
			);
		}
		await waitFor(() =>
			expect(pin).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				"India",
				undefined,
			),
		);
		await openActions("India");
		await screen.findByRole("menuitem", { name: "Edit project" });
		expect(screen.queryByRole("menuitem", { name: "Pin project" })).toBeNull();
	});

	it("unpins once when the pin button is double-clicked", async () => {
		const user = userEvent.setup({ delay: null });
		vi.spyOn(API.experimental, "getChatProjectSummaries").mockResolvedValue([
			summary("Juliet", { pinned: true, pin_order: 1 }),
			summary("Kilo"),
		]);
		const unpin = vi
			.spyOn(API.experimental, "unpinChatProject")
			.mockResolvedValue();
		const pin = vi
			.spyOn(API.experimental, "pinChatProject")
			.mockReturnValue(new Promise(() => {}));
		renderWithAuth(<ProjectsPage />);
		await user.dblClick(
			await screen.findByRole("button", { name: "Unpin Juliet" }),
		);
		// Pin changes run in order, so this pin is sent after any second unpin.
		await user.click(
			screen.getByRole("button", { name: "Open project actions for Kilo" }),
		);
		await user.click(screen.getByRole("menuitem", { name: "Pin project" }));
		await waitFor(() => expect(pin).toHaveBeenCalled());
		expect(unpin).toHaveBeenCalledOnce();
	});

	it("archives a project from its card and moves focus to New project", async () => {
		const user = userEvent.setup();
		// Summaries leave out archived projects.
		vi.spyOn(API.experimental, "getChatProjectSummaries")
			.mockResolvedValueOnce([summary("Kilo")])
			.mockResolvedValue([]);
		vi.spyOn(
			API.experimental,
			"getChatProjectArchivePreview",
		).mockResolvedValue({ chat_count: 0, active_chat_count: 0 });
		const archive = vi
			.spyOn(API.experimental, "archiveChatProject")
			.mockImplementation(async (_organizationId, projectId) => ({
				...MockArchivedChatProject,
				id: projectId,
			}));
		renderWithAuth(<ProjectsPage />);
		await user.click(
			await screen.findByRole("button", {
				name: "Open project actions for Kilo",
			}),
		);
		await user.click(screen.getByRole("menuitem", { name: "Archive project" }));
		await user.click(
			await screen.findByRole("button", { name: "Archive project" }),
		);

		await waitFor(() =>
			expect(archive).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				"Kilo",
				{ stop_active_chats: false },
			),
		);
		await waitFor(() =>
			// The header button, ahead of the empty state's.
			expect(
				screen.getAllByRole("button", { name: "New project" })[0],
			).toHaveFocus(),
		);
	});

	it("unarchives once from the Archived tab, then leaves the emptied tab", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChatProjectSummaries").mockResolvedValue([
			summary("Kilo"),
		]);
		const mockArchivedLima = {
			...MockArchivedChatProject,
			id: "Lima",
			name: "Lima",
		};
		let archived = [mockArchivedLima];
		vi.spyOn(API.experimental, "getChatProjects").mockImplementation(
			async (params) => (params?.archived ? archived : []),
		);
		const unarchived = createDeferred<ChatProject>();
		const unarchive = vi
			.spyOn(API.experimental, "unarchiveChatProject")
			.mockReturnValue(unarchived.promise);
		const { router } = renderWithAuth(<ProjectsPage />, {
			route: "/?tab=archived",
		});
		const pickUnarchive = async () => {
			await user.click(
				await screen.findByRole("button", {
					name: "Open project actions for Lima",
				}),
			);
			await user.click(
				screen.getByRole("menuitem", { name: "Unarchive project" }),
			);
		};

		// The card stays until the request succeeds, so it can be picked twice.
		await pickUnarchive();
		await pickUnarchive();
		archived = [];
		unarchived.resolve({ ...mockArchivedLima, archived_at: undefined });

		await waitFor(() =>
			expect(screen.getByRole("button", { name: "New project" })).toHaveFocus(),
		);
		expect(unarchive).toHaveBeenCalledOnce();
		expect(router.state.location.search).toBe("");
	});

	it("moves focus to New project when Retry loads the projects", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChatProjectSummaries")
			.mockRejectedValueOnce(new Error("boom"))
			.mockResolvedValue([summary("Kilo")]);
		renderWithAuth(<ProjectsPage />);
		await user.click(await screen.findByRole("button", { name: "Retry" }));
		await waitFor(() =>
			expect(screen.getByRole("button", { name: "New project" })).toHaveFocus(),
		);
	});
});
