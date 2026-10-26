import type { ChatArtifact } from "#/api/typesGenerated";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import { MockUserMember, MockUserOwner } from "#/testHelpers/entities";
import { filterArtifacts } from "./ArtifactsGalleryPageView";

const own: ChatArtifact = {
	...MockChatArtifact,
	id: "own",
	title: "Repo layout",
	latest_name: "repo-layout.md",
};
const shared: ChatArtifact = {
	...MockChatArtifact,
	id: "shared",
	title: "Team notes",
	latest_name: "notes.md",
	owner_id: MockUserMember.id,
	owner_username: MockUserMember.username,
	project_id: "project-1",
	project_name: "Billing",
};
const all = [own, shared];
const ids = (artifacts: readonly ChatArtifact[]) => artifacts.map((a) => a.id);

describe("filterArtifacts", () => {
	it.each([
		["all", ["own", "shared"]],
		["yours", ["own"]],
		["shared", ["shared"]],
	] as const)("keeps the %s artifacts", (filter, expected) => {
		expect(ids(filterArtifacts(all, filter, "", MockUserOwner.id))).toEqual(
			expected,
		);
	});

	it.each([
		["the title", "team"],
		["the file name", "LAYOUT.MD"],
		["the project name", "billing"],
		["the owner name", MockUserMember.username.toLowerCase()],
	])("matches %s without case", (_, search) => {
		expect(
			filterArtifacts(all, "all", `  ${search} `, MockUserOwner.id),
		).toHaveLength(1);
	});

	it("applies the filter and the search together", () => {
		expect(filterArtifacts(all, "yours", "team", MockUserOwner.id)).toEqual([]);
	});
});
