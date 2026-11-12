import { screen } from "@testing-library/react";
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
		expect({
			yours: names(projects.yours),
			shared: names(projects.shared),
		}).toEqual({
			yours: [
				"first pin",
				"second pin",
				"newer",
				"Tied",
				"older",
				"Alpha",
				"zeta",
			],
			shared: ["theirs"],
		});
	});
});

describe("ProjectsPage", () => {
	it("holds back Pin for each project whose pin is still pending", async () => {
		const user = userEvent.setup();
		vi.spyOn(API.experimental, "getChatProjectSummaries").mockResolvedValue([
			summary("India"),
			summary("Kilo"),
		]);
		vi.spyOn(API.experimental, "pinChatProject").mockReturnValue(
			new Promise(() => {}),
		);
		renderWithAuth(<ProjectsPage />);
		const openActions = async (name: string) =>
			user.click(
				await screen.findByRole("button", {
					name: `Open project actions for ${name}`,
				}),
			);

		for (const name of ["India", "Kilo"]) {
			await openActions(name);
			await user.click(
				await screen.findByRole("menuitem", { name: "Pin project" }),
			);
		}
		await openActions("India");

		expect(await screen.findByRole("menuitem", { name: /Edit/ })).toBeVisible();
		expect(screen.queryByRole("menuitem", { name: "Pin project" })).toBeNull();
		expect(screen.getByRole("status", { name: "Pinning Kilo" })).toBeVisible();
	});
});
