import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	chatArtifact,
	chatArtifactVersionText,
} from "#/api/queries/chatArtifacts";
import type { ChatArtifact, ChatMessage } from "#/api/typesGenerated";
import {
	MOCK_TIMESTAMP,
	MockChat,
	MockChatArtifact,
} from "#/testHelpers/chatEntities";
import { MockUserOwner } from "#/testHelpers/entities";
import { createChatStore } from "../ChatConversation/chatStore";
import { ArtifactsPanel } from "./ArtifactsPanel";

const MockPdfArtifact: ChatArtifact = {
	...MockChatArtifact,
	id: "artifact-2",
	title: "Quarterly report",
	latest_version: 1,
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

const artifactMessage = (
	id: number,
	toolName: string,
	artifact: ChatArtifact,
): ChatMessage => ({
	id,
	chat_id: MockChat.id,
	created_at: MOCK_TIMESTAMP,
	role: "tool",
	content: [
		{
			type: "tool-result",
			tool_name: toolName,
			result: {
				ok: true,
				artifact_id: artifact.id,
				version: artifact.latest_version,
			},
		},
	],
});

const storeWith = (messages: ChatMessage[]) => {
	const store = createChatStore();
	store.replaceMessages(messages);
	return store;
};

const meta: Meta<typeof ArtifactsPanel> = {
	title: "pages/AgentsPage/Artifacts/ArtifactsPanel",
	component: ArtifactsPanel,
	args: {
		chatId: MockChat.id,
		currentUserId: MockUserOwner.id,
		selection: null,
		onSelect: fn(),
		onPrompt: fn(),
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
type Story = StoryObj<typeof ArtifactsPanel>;

export const Empty: Story = {
	args: { store: storeWith([]) },
};

export const MarkdownVersion: Story = {
	args: {
		store: storeWith([
			artifactMessage(1, "read_artifact", MockPdfArtifact),
			artifactMessage(2, "publish_artifact", MockChatArtifact),
		]),
	},
	parameters: {
		queries: [
			{
				key: chatArtifact(MockChatArtifact.id).queryKey,
				data: MockChatArtifact,
			},
			{ key: chatArtifact(MockPdfArtifact.id).queryKey, data: MockPdfArtifact },
			{
				key: chatArtifactVersionText(MockChatArtifact.id, 2).queryKey,
				data: "# Repo layout\n\n- `coderd/` API server\n- `site/` frontend\n- `enterprise/` paid features\n",
			},
		],
	},
};

export const DownloadVersion: Story = {
	args: {
		store: storeWith([artifactMessage(1, "publish_artifact", MockPdfArtifact)]),
	},
	parameters: {
		queries: [
			{ key: chatArtifact(MockPdfArtifact.id).queryKey, data: MockPdfArtifact },
		],
	},
};
