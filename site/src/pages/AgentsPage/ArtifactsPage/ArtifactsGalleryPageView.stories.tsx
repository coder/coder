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

const mockSharedFields: Partial<ChatArtifact> = {
	owner_id: MockUserMember.id,
	owner_username: MockUserMember.username,
	project_id: "project-1",
	project_name: "Artifacts demo",
};

const mockRepoLayoutArtifact: ChatArtifact = {
	...MockChatArtifact,
	id: "repo-layout",
	versions: undefined,
	latest_version: 4,
	updated_at: "2024-01-08T15:00:00Z",
};

const mockCacheOptionsArtifact: ChatArtifact = {
	...MockChatArtifact,
	id: "cache-options",
	versions: undefined,
	title: "Cache options",
	latest_name: "cache-options.html",
	latest_media_type: "text/plain",
	latest_version: 1,
	updated_at: "2024-01-08T12:00:00Z",
};

const mockTeamNotesArtifact: ChatArtifact = {
	...MockChatArtifact,
	...mockSharedFields,
	id: "team-notes",
	versions: undefined,
	title: "Team notes",
	latest_name: "team-notes.md",
	updated_at: "2024-01-07T12:00:00Z",
};

const mockPublishFlowArtifact: ChatArtifact = {
	...MockChatArtifact,
	id: "publish-flow",
	versions: undefined,
	title: "Publish flow",
	latest_name: "flow.mmd",
	latest_media_type: "text/plain",
	latest_version: 1,
	updated_at: "2024-01-04T12:00:00Z",
};

const mockGradientArtifact: ChatArtifact = {
	...MockChatArtifact,
	...mockSharedFields,
	id: "gradient",
	versions: undefined,
	title: "Gradient",
	latest_name: "gradient.png",
	latest_media_type: "image/png",
	latest_version: 1,
	updated_at: "2023-12-01T12:00:00Z",
};

const mockTestCountsArtifact: ChatArtifact = {
	...MockChatArtifact,
	id: "test-counts",
	versions: undefined,
	title: "Test counts",
	latest_name: "counts.csv",
	latest_media_type: "text/csv",
	latest_version: 1,
	updated_at: "2023-11-20T12:00:00Z",
};

const mockQuarterlyReportArtifact: ChatArtifact = {
	...MockChatArtifact,
	id: "quarterly-report",
	versions: undefined,
	title: "Quarterly report",
	latest_name: "quarterly.pdf",
	latest_media_type: "application/pdf",
	latest_version: 3,
	updated_at: "2023-11-02T12:00:00Z",
};

const mockArtifacts: ChatArtifact[] = [
	mockRepoLayoutArtifact,
	mockCacheOptionsArtifact,
	mockTeamNotesArtifact,
	mockPublishFlowArtifact,
	mockGradientArtifact,
	mockTestCountsArtifact,
	mockQuarterlyReportArtifact,
];

const meta: Meta<typeof ArtifactsGalleryPageView> = {
	title: "pages/AgentsPage/ArtifactsGalleryPageView",
	component: ArtifactsGalleryPageView,
	args: {
		artifacts: mockArtifacts,
		error: undefined,
		currentUserId: MockUserOwner.id,
		now,
	},
	// The layout choice persists in localStorage. Clear it before and after
	// each story, so that only the grid stories show the grid.
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
