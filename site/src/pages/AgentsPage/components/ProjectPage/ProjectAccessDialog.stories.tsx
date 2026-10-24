import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
	MockGroup,
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { ProjectAccessDialog } from "./ProjectAccessDialog";
import { buildAccessEntries } from "./projectAccess";

const mockACL = {
	users: [
		{ ...MockUserMember, role: "admin" as const },
		{
			...MockUserMember,
			id: "sharee-2",
			username: "grace",
			name: "",
			role: "use" as const,
		},
	],
	groups: [{ ...MockGroup, role: "use" as const }],
};

const meta: Meta<typeof ProjectAccessDialog> = {
	title: "pages/AgentsPage/ProjectPage/ProjectAccessDialog",
	component: ProjectAccessDialog,
	args: {
		open: true,
		onOpenChange: fn(),
		entries: buildAccessEntries(MockUserOwner.id, MockUserOwner, mockACL),
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
		entries: buildAccessEntries(MockUserOwner.id, MockUserOwner, {
			users: [],
			groups: [],
		}),
	},
};

export const OwnerLoading: Story = {
	args: {
		entries: buildAccessEntries(MockUserOwner.id, undefined, mockACL),
	},
};

export const OwnerUnknown: Story = {
	args: {
		entries: buildAccessEntries(MockUserOwner.id, null, mockACL),
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
