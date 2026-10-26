import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { MockChatArtifact } from "#/testHelpers/chatEntities";
import { MockUserMember, MockUserOwner } from "#/testHelpers/entities";
import { ArtifactShareButton } from "./ArtifactShareButton";

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
			<div className="flex h-60 w-105 justify-end">
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

export const OlderVersion: Story = {
	args: { version: 1 },
};

export const ProjectArtifact: Story = {
	args: { artifact: mockProjectArtifact },
};

export const AnotherUsersArtifact: Story = {
	args: { currentUserId: MockUserMember.id },
};
