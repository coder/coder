import type { Meta, StoryObj } from "@storybook/react-vite";
import { Outlet } from "react-router";
import { spyOn } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	chatArtifactKey,
	chatArtifactVersionTextKey,
} from "#/api/queries/chatArtifacts";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import { MockUserOwner, mockApiError } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import ArtifactPage from "./ArtifactPage";

const artifactRoute = (artifactId: string) =>
	reactRouterParameters({
		location: {
			path: `/agents/artifacts/${artifactId}`,
			pathParams: { artifactId },
		},
		routing: [
			{
				path: "/",
				element: (
					<Outlet
						context={{
							isSidebarCollapsed: false,
							onExpandSidebar: () => undefined,
						}}
					/>
				),
				children: [
					{ path: "agents/artifacts/:artifactId", useStoryElement: true },
				],
			},
		],
	});

const meta: Meta<typeof ArtifactPage> = {
	title: "pages/AgentsPage/ArtifactPage",
	component: ArtifactPage,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
	},
};

export default meta;
type Story = StoryObj<typeof ArtifactPage>;

const repoLayoutText =
	"# Repo layout\n\n- `coderd/` API server\n- `site/` frontend\n";

export const Loaded: Story = {
	parameters: {
		reactRouter: artifactRoute(MockChatArtifact.id),
		queries: [
			{ key: chatArtifactKey(MockChatArtifact.id), data: MockChatArtifact },
			{
				key: chatArtifactVersionTextKey(MockChatArtifact.id, 2),
				data: repoLayoutText,
			},
		],
	},
};

/** On a phone the header holds the back link and the title takes its own row. */
export const LoadedMobile: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
		reactRouter: artifactRoute(MockChatArtifact.id),
		queries: [
			{ key: chatArtifactKey(MockChatArtifact.id), data: MockChatArtifact },
			{
				key: chatArtifactVersionTextKey(MockChatArtifact.id, 2),
				data: repoLayoutText,
			},
		],
	},
};

const mockArtifactFromAnotherUsersChat = {
	...MockChatArtifact,
	versions: (MockChatArtifact.versions ?? []).map((version, index) =>
		index === 0 ? { ...version, chat_title: "" } : version,
	),
};

/** The viewer does not own the latest version's chat, so "Open chat" is absent. */
export const LatestFromAnotherUsersChat: Story = {
	parameters: {
		reactRouter: artifactRoute(MockChatArtifact.id),
		queries: [
			{
				key: chatArtifactKey(MockChatArtifact.id),
				data: mockArtifactFromAnotherUsersChat,
			},
			{
				key: chatArtifactVersionTextKey(MockChatArtifact.id, 2),
				data: repoLayoutText,
			},
		],
	},
};

export const NotFound: Story = {
	parameters: {
		reactRouter: artifactRoute("missing"),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatArtifact").mockRejectedValue({
			...mockApiError({
				message:
					"Resource not found or you do not have access to this resource",
			}),
			status: 404,
		});
	},
};
