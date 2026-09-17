import type { Meta, StoryObj } from "@storybook/react-vite";
import { IdpSyncEmptyState } from "./IdpSyncEmptyState";

const meta: Meta<typeof IdpSyncEmptyState> = {
	title: "modules/idpSync/IdpSyncEmptyState",
	component: IdpSyncEmptyState,
	args: {
		title: "No IdP organization sync configured",
		description:
			"Automatically assign users to organizations based on their identity provider groups.",
		ctaLabel: "Set up IdP organization sync",
		docsHref: "https://coder.com/docs/admin/users/idp-sync#organization-sync",
	},
};

export default meta;
type Story = StoryObj<typeof IdpSyncEmptyState>;

export const OrganizationMapping: Story = {};

export const GroupMapping: Story = {
	args: {
		title: "No IdP group sync configured",
		description:
			"Automatically assign users to groups based on their identity provider claims.",
		ctaLabel: "Set up IdP group sync",
		docsHref: "https://coder.com/docs/admin/users/idp-sync#group-sync",
	},
};

export const RoleMapping: Story = {
	args: {
		title: "No IdP role sync configured",
		description:
			"Automatically assign roles to users based on their identity provider claims.",
		ctaLabel: "Set up IdP role sync",
		docsHref: "https://coder.com/docs/admin/users/idp-sync#role-sync",
	},
};
