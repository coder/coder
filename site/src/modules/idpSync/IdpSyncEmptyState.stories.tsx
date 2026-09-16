import type { Meta, StoryObj } from "@storybook/react-vite";
import { IdpSyncEmptyState } from "./IdpSyncEmptyState";

const meta: Meta<typeof IdpSyncEmptyState> = {
	title: "modules/idpSync/IdpSyncEmptyState",
	component: IdpSyncEmptyState,
	args: {
		title: "Set up organization mapping",
		description:
			"Automatically assign users to organizations based on their identity provider groups.",
		docsHref: "https://coder.com/docs/admin/users/idp-sync#organization-sync",
	},
};

export default meta;
type Story = StoryObj<typeof IdpSyncEmptyState>;

export const OrganizationMapping: Story = {};

export const GroupMapping: Story = {
	args: {
		title: "Set up group mapping",
		description:
			"Automatically assign users to groups based on their identity provider claims.",
		docsHref: "https://coder.com/docs/admin/users/idp-sync#group-sync",
	},
};

export const RoleMapping: Story = {
	args: {
		title: "Set up role mapping",
		description:
			"Automatically assign roles to users based on their identity provider claims.",
		docsHref: "https://coder.com/docs/admin/users/idp-sync#role-sync",
	},
};
