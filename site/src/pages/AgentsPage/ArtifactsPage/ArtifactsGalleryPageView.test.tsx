import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createMemoryRouter } from "react-router";
import type { ChatArtifact } from "#/api/typesGenerated";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import { MockUserMember, MockUserOwner } from "#/testHelpers/entities";
import { render, renderWithRouter } from "#/testHelpers/renderHelpers";
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

describe("filterArtifacts", () => {
	it.each([
		["all", ["own", "shared"]],
		["yours", ["own"]],
		["shared", ["shared"]],
	] as const)("keeps the %s artifacts", (filter, expected) => {
		expect(
			filterArtifacts(mockArtifacts, filter, "", MockUserOwner.id).map(
				(artifact) => artifact.id,
			),
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

	const renderGallery = (route: string) =>
		renderWithRouter(
			createMemoryRouter(
				[
					{
						path: "/agents/artifacts",
						element: (
							<ArtifactsGalleryPageView
								artifacts={mockArtifacts}
								error={undefined}
								currentUserId={MockUserOwner.id}
								now={new Date(MockChatArtifact.updated_at)}
							/>
						),
					},
					{ path: "/agents/artifacts/:artifactId", element: <div /> },
				],
				{ initialEntries: [route] },
			),
		);

	it("keeps the filter and the search in the URL", async () => {
		const user = userEvent.setup();
		const { router } = renderGallery("/agents/artifacts?pr_status=open");

		await user.click(await screen.findByRole("button", { name: "Yours" }));
		await user.type(
			screen.getByRole("textbox", { name: "Search artifacts" }),
			"repo",
		);

		expect(
			Object.fromEntries(new URLSearchParams(router.state.location.search)),
		).toEqual({ pr_status: "open", show: "yours", search: "repo" });
	});

	it("shows the artifacts that the URL filter and search select", async () => {
		renderGallery("/agents/artifacts?show=shared&search=notes");

		expect(
			screen.getByRole("textbox", { name: "Search artifacts" }),
		).toHaveValue("notes");
		expect(
			(await screen.findAllByRole("link")).map((link) =>
				link.getAttribute("href"),
			),
		).toEqual([`/agents/artifacts/${mockSharedArtifact.id}`]);
	});

	it("passes the gallery URL to the artifact page", async () => {
		const user = userEvent.setup();
		const { router } = renderGallery("/agents/artifacts?show=yours");

		await user.click(await screen.findByRole("link", { name: /Repo layout/ }));

		expect(router.state.location.pathname).toBe(
			`/agents/artifacts/${mockOwnArtifact.id}`,
		);
		expect(router.state.location.state).toEqual({
			from: "/agents/artifacts?show=yours",
		});
	});
});
