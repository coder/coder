import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import { chatArtifactACLKey } from "#/api/queries/chatArtifacts";
import {
	MockChatArtifact,
	MockChatGroup,
	MockChatUser,
} from "#/testHelpers/chatEntities";
import { MockUserMember, MockUserOwner } from "#/testHelpers/entities";
import { ArtifactShareButton } from "./ArtifactShareButton";
import { getArtifactShareCheck } from "./artifactUtils";

const mockProjectArtifact = {
	...MockChatArtifact,
	project_id: "project-1",
	project_name: "Artifacts demo",
};

const meta: Meta<typeof ArtifactShareButton> = {
	title: "pages/AgentsPage/Artifacts/ArtifactShareButton",
	component: ArtifactShareButton,
	args: {
		artifact: MockChatArtifact,
		version: MockChatArtifact.latest_version,
		currentUserId: MockUserOwner.id,
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

export const OwnArtifact: Story = {
	parameters: {
		queries: [
			{
				key: getAuthorizationKey(getArtifactShareCheck(MockChatArtifact)),
				data: { canShare: true },
			},
			{
				key: chatArtifactACLKey(MockChatArtifact.id),
				data: { users: [], groups: [] },
			},
		],
	},
};

export const SharedWithPeople: Story = {
	parameters: {
		queries: [
			{
				key: getAuthorizationKey(getArtifactShareCheck(MockChatArtifact)),
				data: { canShare: true },
			},
			{
				key: chatArtifactACLKey(MockChatArtifact.id),
				data: { users: [MockChatUser], groups: [MockChatGroup] },
			},
		],
	},
};

export const OlderVersion: Story = {
	args: { version: 1 },
	parameters: {
		queries: [
			{
				key: getAuthorizationKey(getArtifactShareCheck(MockChatArtifact)),
				data: { canShare: true },
			},
			{
				key: chatArtifactACLKey(MockChatArtifact.id),
				data: { users: [], groups: [] },
			},
		],
	},
};

export const ProjectArtifact: Story = {
	args: { artifact: mockProjectArtifact },
	parameters: {
		queries: [
			{
				key: getAuthorizationKey(getArtifactShareCheck(mockProjectArtifact)),
				data: { canShare: true },
			},
			{
				key: chatArtifactACLKey(mockProjectArtifact.id),
				data: { users: [], groups: [] },
			},
		],
	},
};

export const AnotherUsersArtifact: Story = {
	args: { currentUserId: MockUserMember.id },
	parameters: {
		queries: [
			{
				key: getAuthorizationKey(getArtifactShareCheck(MockChatArtifact)),
				data: { canShare: false },
			},
		],
	},
};
