import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { ChatArtifact } from "#/api/typesGenerated";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import {
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import {
	ArtifactsGalleryPageView,
	galleryLayoutStorageKey,
} from "./ArtifactsGalleryPageView";

const now = new Date("2024-01-08T18:00:00Z");

const artifact = (
	id: string,
	title: string,
	latestName: string,
	latestMediaType: string,
	updatedAt: string,
	extra: Partial<ChatArtifact> = {},
): ChatArtifact => ({
	...MockChatArtifact,
	id,
	title,
	latest_name: latestName,
	latest_media_type: latestMediaType,
	updated_at: updatedAt,
	versions: undefined,
	...extra,
});

const sharedByMember: Partial<ChatArtifact> = {
	owner_id: MockUserMember.id,
	owner_username: MockUserMember.username,
	project_id: "project-1",
	project_name: "Artifacts demo",
};

const artifacts: ChatArtifact[] = [
	artifact(
		"a1",
		"Repo layout",
		"repo-layout.md",
		"text/markdown",
		"2024-01-08T15:00:00Z",
		{ latest_version: 4 },
	),
	artifact(
		"a2",
		"Cache options",
		"cache-options.html",
		"text/plain",
		"2024-01-08T12:00:00Z",
		{ latest_version: 1 },
	),
	artifact(
		"a3",
		"Team notes",
		"team-notes.md",
		"text/markdown",
		"2024-01-07T12:00:00Z",
		sharedByMember,
	),
	artifact(
		"a4",
		"Publish flow",
		"flow.mmd",
		"text/plain",
		"2024-01-04T12:00:00Z",
		{ latest_version: 1 },
	),
	artifact(
		"a5",
		"Gradient",
		"gradient.png",
		"image/png",
		"2023-12-01T12:00:00Z",
		{ ...sharedByMember, latest_version: 1 },
	),
	artifact(
		"a6",
		"Test counts",
		"counts.csv",
		"text/csv",
		"2023-11-20T12:00:00Z",
		{ latest_version: 1 },
	),
	artifact(
		"a7",
		"Quarterly report",
		"quarterly.pdf",
		"application/pdf",
		"2023-11-02T12:00:00Z",
		{ latest_version: 3 },
	),
];

const meta: Meta<typeof ArtifactsGalleryPageView> = {
	title: "pages/AgentsPage/ArtifactsGalleryPageView",
	component: ArtifactsGalleryPageView,
	args: {
		artifacts,
		error: undefined,
		currentUserId: MockUserOwner.id,
		now,
	},
	// The layout choice is stored, so each story starts from the list.
	beforeEach: () => {
		localStorage.removeItem(galleryLayoutStorageKey);
		return () => localStorage.removeItem(galleryLayoutStorageKey);
	},
};

export default meta;
type Story = StoryObj<typeof ArtifactsGalleryPageView>;

export const List: Story = {};

export const Grid: Story = {
	beforeEach: () => {
		localStorage.setItem(galleryLayoutStorageKey, "grid");
	},
};

export const SharedWithYou: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Shared with you" }),
		);
	},
};

export const Search: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.type(
			within(canvasElement).getByRole("textbox", { name: "Search artifacts" }),
			"notes",
		);
	},
};

export const NoMatches: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.type(
			within(canvasElement).getByRole("textbox", { name: "Search artifacts" }),
			"roadmap",
		);
	},
};

export const Empty: Story = {
	args: { artifacts: [] },
};

export const Loading: Story = {
	args: { artifacts: undefined },
};

export const LoadError: Story = {
	args: {
		artifacts: undefined,
		error: mockApiError({ message: "Failed to list chat artifacts." }),
	},
};

export const MobileList: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
};

export const MobileGrid: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
	beforeEach: () => {
		localStorage.setItem(galleryLayoutStorageKey, "grid");
	},
};
