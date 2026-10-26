import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ChatArtifact } from "#/api/typesGenerated";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import { MockUserMember, MockUserOwner } from "#/testHelpers/entities";
import { render } from "#/testHelpers/renderHelpers";
import {
	ArtifactsGalleryPageView,
	filterArtifacts,
	galleryLayoutStorageKey,
} from "./ArtifactsGalleryPageView";

const mockOwnArtifact: ChatArtifact = {
	...MockChatArtifact,
	id: "own",
	title: "Repo layout",
	latest_name: "repo-layout.md",
};
const mockSharedArtifact: ChatArtifact = {
	...MockChatArtifact,
	id: "shared",
	title: "Team notes",
	latest_name: "notes.md",
	owner_id: MockUserMember.id,
	owner_username: MockUserMember.username,
	project_id: "project-1",
	project_name: "Billing",
};
const mockArtifacts = [mockOwnArtifact, mockSharedArtifact];
const ids = (artifacts: readonly ChatArtifact[]) => artifacts.map((a) => a.id);

describe("filterArtifacts", () => {
	it.each([
		["all", ["own", "shared"]],
		["yours", ["own"]],
		["shared", ["shared"]],
	] as const)("keeps the %s artifacts", (filter, expected) => {
		expect(
			ids(filterArtifacts(mockArtifacts, filter, "", MockUserOwner.id)),
		).toEqual(expected);
	});

	it.each([
		["the title", "team"],
		["the file name", "LAYOUT.MD"],
		["the project name", "billing"],
		["the owner name", MockUserMember.username.toLowerCase()],
	])("matches %s without case", (_, search) => {
		expect(
			filterArtifacts(mockArtifacts, "all", `  ${search} `, MockUserOwner.id),
		).toHaveLength(1);
	});

	it("applies the filter and the search together", () => {
		expect(
			filterArtifacts(mockArtifacts, "yours", "team", MockUserOwner.id),
		).toEqual([]);
	});
});

describe("ArtifactsGalleryPageView", () => {
	beforeEach(() => localStorage.removeItem(galleryLayoutStorageKey));
	afterEach(() => localStorage.removeItem(galleryLayoutStorageKey));

	it("stores the layout that the user picks", async () => {
		const user = userEvent.setup();
		render(
			<ArtifactsGalleryPageView
				artifacts={mockArtifacts}
				error={undefined}
				currentUserId={MockUserOwner.id}
				now={new Date(MockChatArtifact.updated_at)}
			/>,
		);

		await user.click(await screen.findByRole("button", { name: "Grid view" }));
		expect(localStorage.getItem(galleryLayoutStorageKey)).toBe("grid");

		await user.click(screen.getByRole("button", { name: "List view" }));
		expect(localStorage.getItem(galleryLayoutStorageKey)).toBe("list");
	});
});
