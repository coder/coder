import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import type { ChatProjectSummary } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectSummary,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import ProjectsPage, { getProjectsByTab } from "./ProjectsPage";

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

const names = (summaries: readonly ChatProjectSummary[]) =>
	summaries.map((s) => s.project.name);

describe("getProjectsByTab", () => {
	it("splits by owner and orders pins, then activity, then no chats", () => {
		const earlier = "2026-11-01T00:00:00Z";
		const later = "2026-11-02T00:00:00Z";
		const projects = getProjectsByTab(
			[
				summary("zeta"),
				summary("Alpha"),
				summary("theirs", {}, MockUserMember.id),
				summary("older", { last_activity_at: earlier }),
				summary("second pin", { pinned: true, pin_order: 2 }),
				summary("newer", { last_activity_at: later }),
				summary("Tied", { last_activity_at: later }),
				summary("first pin", { pinned: true, pin_order: 1 }),
			],
			MockUserOwner.id,
		);
		expect(names(projects.shared)).toEqual(["theirs"]);
		expect(names(projects.yours)).toEqual([
			"first pin",
			"second pin",
			"newer",
			"Tied",
			"older",
			"Alpha",
			"zeta",
		]);
	});
});

describe("ProjectsPage", () => {
	it("pins a project at the end from its card's actions menu", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChatProjectSummaries").mockResolvedValue([
			summary("India"),
		]);
		const pin = vi
			.spyOn(API.experimental, "pinChatProject")
			.mockReturnValue(new Promise(() => {}));
		renderWithAuth(<ProjectsPage />);

		await user.click(
			await screen.findByRole("button", {
				name: "Open project actions for India",
			}),
		);
		await user.click(
			await screen.findByRole("menuitem", { name: "Pin project" }),
		);

		await waitFor(() =>
			expect(pin).toHaveBeenCalledWith(
				MockChatProject.organization_id,
				"India",
				undefined,
			),
		);
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
