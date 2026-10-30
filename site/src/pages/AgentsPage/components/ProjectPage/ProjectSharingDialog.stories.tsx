import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn } from "storybook/test";
import { API } from "#/api/api";
import { chatProjectACLAvailable } from "#/api/queries/chatProjects";
import {
	MockChatModelACLAvailable,
	MockChatProject,
	MockChatProjectACL,
	mockApiError,
} from "#/testHelpers/entities";
import { ProjectSharingDialogView } from "./ProjectSharingDialog";

const meta: Meta<typeof ProjectSharingDialogView> = {
	title: "pages/AgentsPage/ProjectPage/ProjectSharingDialog",
	component: ProjectSharingDialogView,
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectACLAvailable").mockResolvedValue(
			MockChatModelACLAvailable,
		);
	},
	args: {
		project: MockChatProject,
		acl: MockChatProjectACL,
		loadError: undefined,
		saveError: undefined,
		isSaving: false,
		availableQueryOptions: (options) =>
			chatProjectACLAvailable(MockChatProject, options),
		onAdd: fn(),
		onChangeRole: fn(),
		onRemove: fn(),
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ProjectSharingDialogView>;

export const Shared: Story = {};

export const NotShared: Story = {
	args: { acl: { users: [], groups: [] } },
};

export const Loading: Story = { args: { acl: undefined } };

export const LoadError: Story = {
	args: {
		acl: undefined,
		loadError: mockApiError({ message: "Failed to load sharing." }),
	},
};

export const SharingDisabled: Story = {
	args: {
		saveError: mockApiError({
			message: "Sharing is disabled for this deployment.",
		}),
	},
};
