import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { chatArtifactACLKey } from "#/api/queries/chatArtifacts";
import type { ChatGroup, ChatUser } from "#/api/typesGenerated";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import {
	MockGroup,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { ArtifactShareButton } from "./ArtifactShareButton";

const mockProjectArtifact = {
	...MockChatArtifact,
	project_id: "project-1",
	project_name: "Artifacts demo",
};

const mockSharedUser: ChatUser = {
	id: MockUserMember.id,
	username: MockUserMember.username,
	name: MockUserMember.name,
	avatar_url: MockUserMember.avatar_url,
	role: "read",
};

const mockSharedGroup: ChatGroup = { ...MockGroup, role: "read" };

const meta: Meta<typeof ArtifactShareButton> = {
	title: "pages/AgentsPage/Artifacts/ArtifactShareButton",
	component: ArtifactShareButton,
	args: {
		artifact: MockChatArtifact,
		version: MockChatArtifact.latest_version,
		currentUserId: MockUserOwner.id,
	},
	parameters: {
		queries: [
			{
				key: chatArtifactACLKey(MockChatArtifact.id),
				data: { users: [], groups: [] },
			},
		],
	},
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockResolvedValue({ canShare: true });
	},
	decorators: [
		(Story) => (
			<div className="flex h-96 w-105 justify-end">
				<Story />
			</div>
		),
	],
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Share artifact" }),
		);
	},
};

export default meta;
type Story = StoryObj<typeof ArtifactShareButton>;

export const OwnArtifact: Story = {};

export const SharedWithPeople: Story = {
	parameters: {
		queries: [
			{
				key: chatArtifactACLKey(MockChatArtifact.id),
				data: { users: [mockSharedUser], groups: [mockSharedGroup] },
			},
		],
	},
};

export const OlderVersion: Story = {
	args: { version: 1 },
};

export const ProjectArtifact: Story = {
	args: { artifact: mockProjectArtifact },
	parameters: {
		queries: [
			{
				key: chatArtifactACLKey(mockProjectArtifact.id),
				data: { users: [], groups: [] },
			},
		],
	},
};

export const AnotherUsersArtifact: Story = {
	args: { currentUserId: MockUserMember.id },
	beforeEach: () => {
		spyOn(API, "checkAuthorization").mockResolvedValue({ canShare: false });
	},
};
