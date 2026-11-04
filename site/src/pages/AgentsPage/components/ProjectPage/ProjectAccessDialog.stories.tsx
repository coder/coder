import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import type { ChatProjectACL } from "#/api/typesGenerated";
import {
	MockChatProjectACL,
	MockUserMember,
	mockApiError,
} from "#/testHelpers/entities";
import { ProjectAccessDialog } from "./ProjectAccessDialog";
import { buildAccessEntries } from "./projectAccess";

const mockACL: ChatProjectACL = {
	...MockChatProjectACL,
	users: [
		...MockChatProjectACL.users,
		{
			...MockUserMember,
			id: "sharee-2",
			username: "grace",
			name: "",
			role: "use",
		},
	],
};

const meta: Meta<typeof ProjectAccessDialog> = {
	title: "pages/AgentsPage/ProjectPage/ProjectAccessDialog",
	component: ProjectAccessDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		entries: buildAccessEntries(mockACL),
		error: undefined,
		isRetrying: false,
		onRetry: fn(),
		onCloseAutoFocus: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof ProjectAccessDialog>;

export const Default: Story = {};

export const OwnerOnly: Story = {
	args: {
		entries: buildAccessEntries({ ...mockACL, users: [], groups: [] }),
	},
};

export const Loading: Story = {
	args: { entries: undefined },
};

export const LoadError: Story = {
	args: {
		entries: undefined,
		error: mockApiError({ message: "Failed to get chat project ACL." }),
	},
};

export const Retrying: Story = {
	args: {
		entries: undefined,
		error: mockApiError({ message: "Failed to get chat project ACL." }),
		isRetrying: true,
	},
};

/** A failed refetch keeps the cached access list and shows the error. */
export const RefetchError: Story = {
	args: {
		error: mockApiError({ message: "Failed to get chat project ACL." }),
	},
};
