import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn } from "storybook/test";
import { API } from "#/api/api";
import {
	chatArtifactKey,
	chatArtifactVersionTextKey,
} from "#/api/queries/chatArtifacts";
import type { ChatArtifact } from "#/api/typesGenerated";
import {
	MOCK_TIMESTAMP,
	MockChat,
	MockChatArtifact,
} from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { createChatStore } from "../ChatConversation/chatStore";
import { ArtifactPanel } from "./ArtifactPanel";

const MockPdfArtifact: ChatArtifact = {
	...MockChatArtifact,
	id: "artifact-2",
	title: "Quarterly report",
	latest_version: 1,
	latest_name: "quarterly.pdf",
	latest_media_type: "application/pdf",
	versions: [
		{
			version: 1,
			label: "",
			name: "quarterly.pdf",
			media_type: "application/pdf",
			size: 2048,
			created_by_username: MockChatArtifact.owner_username,
			chat_id: MockChat.id,
			chat_title: MockChat.title,
			created_at: MOCK_TIMESTAMP,
		},
	],
};

const meta: Meta<typeof ArtifactPanel> = {
	title: "pages/AgentsPage/Artifacts/ArtifactPanel",
	component: ArtifactPanel,
	args: {
		artifactId: MockChatArtifact.id,
		version: undefined,
		onSelectVersion: fn(),
		chatId: MockChat.id,
		store: createChatStore(),
		onPrompt: fn(),
		currentUserId: MockUserOwner.id,
	},
	decorators: [
		(Story) => (
			<div className="flex h-[560px] w-[480px] flex-col border border-solid border-border">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof ArtifactPanel>;

export const MarkdownVersion: Story = {
	parameters: {
		queries: [
			{ key: chatArtifactKey(MockChatArtifact.id), data: MockChatArtifact },
			{
				key: chatArtifactVersionTextKey(MockChatArtifact.id, 2),
				data: "# Repo layout\n\n- `coderd/` API server\n- `site/` frontend\n- `enterprise/` paid features\n",
			},
		],
	},
};

export const DownloadVersion: Story = {
	args: { artifactId: MockPdfArtifact.id },
	parameters: {
		queries: [
			{ key: chatArtifactKey(MockPdfArtifact.id), data: MockPdfArtifact },
		],
	},
};

export const UnavailableArtifact: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatArtifact").mockRejectedValue(
			new Error("Artifact not found."),
		);
	},
};

export const LoadingArtifact: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatArtifact").mockReturnValue(
			new Promise(() => {}),
		);
	},
};
