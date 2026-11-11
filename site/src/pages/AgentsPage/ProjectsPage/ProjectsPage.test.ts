import { describe, expect, it } from "vitest";
import type { ChatProjectSummary } from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectSummary,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { getProjectsByTab } from "./ProjectsPage";

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
